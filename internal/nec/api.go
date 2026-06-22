package nec

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// APIBaseURL is the data.go.kr OpenAPI gateway for NEC services (org 9760000).
// Unlike the file-dataset backends this needs a serviceKey (활용신청 발급), but
// the application is auto-approved (심의여부: 자동승인) so it stays keyless in
// spirit: any user can obtain one without human review. Keys are secret — the
// caller passes one in and kvote never logs or persists it.
const APIBaseURL = "https://apis.data.go.kr/9760000"

// apiMaxRows is the server-side page cap: numOfRows is silently clamped to 100
// regardless of the requested value, so list operations must paginate.
const apiMaxRows = 100

// VoteSttusInfoPath is the 투·개표(투표율) operation under VoteXmntckInfoInqireService2.
const voteSttusPath = "/VoteXmntckInfoInqireService2/getVoteSttusInfoInqire"

// TurnoutRecord is one region's turnout row from getVoteSttusInfoInqire.
//
// The values are preserved losslessly from the API. sido/gusigun rename the
// opaque sdName/wiwName ("합계" denotes a total: sido="합계" is the nationwide
// total, gusigun="합계" a sido subtotal). electorate/votes/turnout are the
// standard derived dimensions (turnout is supplied by the API and equals
// votes/electorate). The ps*/psEtc* fields are the API's own two-way split of
// the electorate and the vote count; their exact meaning is defined by the API
// guide, so kvote keeps them under their source names rather than guessing a
// label — interpretation is the consumer's.
type TurnoutRecord struct {
	SgID       string  `json:"sgId"`
	SgTypecode string  `json:"sgTypecode"`
	Sido       string  `json:"sido"`
	Gusigun    string  `json:"gusigun"`
	Electorate int     `json:"electorate"`
	Votes      int     `json:"votes"`
	Turnout    float64 `json:"turnout"`
	PsSunsu    int     `json:"psSunsu"`
	PsEtcSunsu int     `json:"psEtcSunsu"`
	PsTusu     int     `json:"psTusu"`
	PsEtcTusu  int     `json:"psEtcTusu"`
}

// apiEnvelope is the common data.go.kr OpenAPI response shape.
type apiEnvelope struct {
	Header struct {
		ResultCode string `xml:"resultCode"`
		ResultMsg  string `xml:"resultMsg"`
	} `xml:"header"`
	Body struct {
		Items struct {
			Item []voteItem `xml:"item"`
		} `xml:"items"`
		NumOfRows  int `xml:"numOfRows"`
		PageNo     int `xml:"pageNo"`
		TotalCount int `xml:"totalCount"`
	} `xml:"body"`
}

type voteItem struct {
	SgID       string  `xml:"sgId"`
	SgTypecode string  `xml:"sgTypecode"`
	SdName     string  `xml:"sdName"`
	WiwName    string  `xml:"wiwName"`
	TotSunsu   int     `xml:"totSunsu"`
	PsSunsu    int     `xml:"psSunsu"`
	PsEtcSunsu int     `xml:"psEtcSunsu"`
	TotTusu    int     `xml:"totTusu"`
	PsTusu     int     `xml:"psTusu"`
	PsEtcTusu  int     `xml:"psEtcTusu"`
	Turnout    float64 `xml:"turnout"`
}

// Turnout fetches every turnout row for one election (sgId) and type
// (sgTypecode), paginating past the 100-row server cap. serviceKey is the
// data.go.kr 일반 인증키; it is sent only as a query parameter and never logged.
//
// An empty result with no error means the API has no data for that
// election/type yet (resultCode INFO-03) — common for an election whose
// numbers the NEC has not finished publishing.
func (c *Client) Turnout(ctx context.Context, serviceKey, sgID, sgType string) ([]TurnoutRecord, error) {
	var out []TurnoutRecord
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("serviceKey", serviceKey)
		q.Set("sgId", sgID)
		q.Set("sgTypecode", sgType)
		q.Set("numOfRows", strconv.Itoa(apiMaxRows))
		q.Set("pageNo", strconv.Itoa(page))

		env, err := c.apiGet(ctx, voteSttusPath, q)
		if err != nil {
			return nil, err
		}
		switch env.Header.ResultCode {
		case "INFO-00", "00", "": // success
		case "INFO-03": // 데이터 없음 — not an error
			return out, nil
		default:
			return nil, fmt.Errorf("openapi %s: %s (%s)", sgID, env.Header.ResultMsg, env.Header.ResultCode)
		}

		for _, it := range env.Body.Items.Item {
			out = append(out, TurnoutRecord{
				SgID:       it.SgID,
				SgTypecode: it.SgTypecode,
				Sido:       it.SdName,
				Gusigun:    it.WiwName,
				Electorate: it.TotSunsu,
				Votes:      it.TotTusu,
				Turnout:    it.Turnout,
				PsSunsu:    it.PsSunsu,
				PsEtcSunsu: it.PsEtcSunsu,
				PsTusu:     it.PsTusu,
				PsEtcTusu:  it.PsEtcTusu,
			})
		}
		if len(out) >= env.Body.TotalCount || len(env.Body.Items.Item) == 0 {
			return out, nil
		}
	}
}

// apiGet performs a throttled GET against the OpenAPI gateway and decodes the
// XML envelope. It uses a distinct base host (apis.data.go.kr) from the
// file-dataset backends, overridable via WithAPIBaseURL for tests.
func (c *Client) apiGet(ctx context.Context, path string, q url.Values) (*apiEnvelope, error) {
	c.throttle()

	u := c.apiBaseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/xml,text/xml,*/*")

	resp, err := c.http.Do(req)
	if err != nil {
		// Avoid leaking the serviceKey (in u) into error output.
		return nil, fmt.Errorf("openapi GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openapi read %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openapi GET %s: status %s", path, resp.Status)
	}
	var env apiEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("openapi parse %s: %w", path, err)
	}
	return &env, nil
}

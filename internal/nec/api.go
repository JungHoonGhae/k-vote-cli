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
	items, err := fetchPages[voteItem](c, ctx, voteSttusPath, serviceKey, sgID, sgType)
	if err != nil {
		return nil, err
	}
	out := make([]TurnoutRecord, 0, len(items))
	for _, it := range items {
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
	return out, nil
}

// winnerPath is the 당선인 operation under WinnerInfoInqireService2.
const winnerPath = "/WinnerInfoInqireService2/getWinnerInfoInqire"

// WinnerRecord is one 당선인(winner) from getWinnerInfoInqire. Fields are
// preserved losslessly from the API; only the opaque source names are renamed.
// Votes/VoteRate are the API's own dugsu/dugyul (득표수/득표율).
type WinnerRecord struct {
	SgID       string  `json:"sgId"`
	SgTypecode string  `json:"sgTypecode"`
	Sido       string  `json:"sido"`    // sdName
	Sgg        string  `json:"sgg"`     // sggName (선거구)
	Gusigun    string  `json:"gusigun"` // wiwName
	Giho       string  `json:"giho"`    // 기호
	Party      string  `json:"party"`   // jdName (정당)
	Name       string  `json:"name"`
	HanjaName  string  `json:"hanjaName"`
	Gender     string  `json:"gender"`
	Birthday   string  `json:"birthday"`
	Age        string  `json:"age"`
	Addr       string  `json:"addr"`
	Job        string  `json:"job"`
	Edu        string  `json:"edu"`
	Career1    string  `json:"career1"`
	Career2    string  `json:"career2"`
	Votes      int     `json:"votes"`    // dugsu (득표수)
	VoteRate   float64 `json:"voteRate"` // dugyul (득표율)
}

type winnerItem struct {
	SgID       string  `xml:"sgId"`
	SgTypecode string  `xml:"sgTypecode"`
	SggName    string  `xml:"sggName"`
	SdName     string  `xml:"sdName"`
	WiwName    string  `xml:"wiwName"`
	Giho       string  `xml:"giho"`
	JdName     string  `xml:"jdName"`
	Name       string  `xml:"name"`
	HanjaName  string  `xml:"hanjaName"`
	Gender     string  `xml:"gender"`
	Birthday   string  `xml:"birthday"`
	Age        string  `xml:"age"`
	Addr       string  `xml:"addr"`
	Job        string  `xml:"job"`
	Edu        string  `xml:"edu"`
	Career1    string  `xml:"career1"`
	Career2    string  `xml:"career2"`
	Dugsu      int     `xml:"dugsu"`
	Dugyul     float64 `xml:"dugyul"`
}

// Winners fetches every 당선인 for one election (sgId) and type (sgTypecode).
// Like Turnout, an empty result with no error means the API has no data yet.
func (c *Client) Winners(ctx context.Context, serviceKey, sgID, sgType string) ([]WinnerRecord, error) {
	items, err := fetchPages[winnerItem](c, ctx, winnerPath, serviceKey, sgID, sgType)
	if err != nil {
		return nil, err
	}
	out := make([]WinnerRecord, 0, len(items))
	for _, it := range items {
		out = append(out, WinnerRecord{
			SgID: it.SgID, SgTypecode: it.SgTypecode,
			Sido: it.SdName, Sgg: it.SggName, Gusigun: it.WiwName,
			Giho: it.Giho, Party: it.JdName, Name: it.Name, HanjaName: it.HanjaName,
			Gender: it.Gender, Birthday: it.Birthday, Age: it.Age, Addr: it.Addr,
			Job: it.Job, Edu: it.Edu, Career1: it.Career1, Career2: it.Career2,
			Votes: it.Dugsu, VoteRate: it.Dugyul,
		})
	}
	return out, nil
}

// apiHeader is the common result header.
type apiHeader struct {
	ResultCode string `xml:"resultCode"`
	ResultMsg  string `xml:"resultMsg"`
}

// apiEnvelope is the generic data.go.kr OpenAPI response shape.
type apiEnvelope[T any] struct {
	Header apiHeader `xml:"header"`
	Body   struct {
		Items struct {
			Item []T `xml:"item"`
		} `xml:"items"`
		TotalCount int `xml:"totalCount"`
	} `xml:"body"`
}

// fetchPages fetches all <item> rows of type T for one election across pages,
// past the 100-row server cap. INFO-03 (no data) yields an empty slice, not an
// error.
func fetchPages[T any](c *Client, ctx context.Context, path, serviceKey, sgID, sgType string) ([]T, error) {
	var out []T
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("serviceKey", serviceKey)
		q.Set("sgId", sgID)
		q.Set("sgTypecode", sgType)
		q.Set("numOfRows", strconv.Itoa(apiMaxRows))
		q.Set("pageNo", strconv.Itoa(page))

		env, err := apiCall[T](c, ctx, path, q)
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
		out = append(out, env.Body.Items.Item...)
		if len(out) >= env.Body.TotalCount || len(env.Body.Items.Item) == 0 {
			return out, nil
		}
	}
}

// apiCall performs a throttled GET against the OpenAPI gateway and decodes the
// generic XML envelope. It uses a distinct base host (apis.data.go.kr) from the
// file-dataset backends, overridable via WithAPIBaseURL for tests.
func apiCall[T any](c *Client, ctx context.Context, path string, q url.Values) (*apiEnvelope[T], error) {
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
	var env apiEnvelope[T]
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("openapi parse %s: %w", path, err)
	}
	return &env, nil
}

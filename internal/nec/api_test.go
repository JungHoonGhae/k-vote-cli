package nec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// emptyEnvelope is a well-formed response with no items, used to terminate
// pagination on page 2+ (the real API returns rows until exhausted).
const emptyEnvelope = `<?xml version="1.0" encoding="UTF-8"?><response>
<header><resultCode>INFO-00</resultCode><resultMsg>NORMAL SERVICE</resultMsg></header>
<body><items></items><numOfRows>100</numOfRows><pageNo>2</pageNo><totalCount>271</totalCount></body>
</response>`

func apiServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/api_turnout.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var keyHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("serviceKey") == "TESTKEY" {
			keyHits++
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("pageNo") == "1" {
			w.Write(fixture)
			return
		}
		w.Write([]byte(emptyEnvelope))
	}))
	t.Cleanup(srv.Close)
	return srv, &keyHits
}

func TestTurnout(t *testing.T) {
	srv, keyHits := apiServer(t)
	c := New(WithAPIBaseURL(srv.URL), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))

	recs, err := c.Turnout(context.Background(), "TESTKEY", "20250603", "1")
	if err != nil {
		t.Fatalf("Turnout: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if *keyHits == 0 {
		t.Fatal("serviceKey was not sent to the server")
	}

	// 합계 행 (전국 집계): 필드 매핑·losslessness 검증.
	tot := recs[0]
	if tot.Sido != "합계" || tot.Gusigun != "합계" {
		t.Errorf("totals row sido/gusigun = %q/%q, want 합계/합계", tot.Sido, tot.Gusigun)
	}
	if tot.Electorate != 44391871 || tot.Votes != 35236497 {
		t.Errorf("electorate/votes = %d/%d, want 44391871/35236497", tot.Electorate, tot.Votes)
	}
	if tot.Turnout != 79.4 {
		t.Errorf("turnout = %v, want 79.4", tot.Turnout)
	}
	// ps*/psEtc* 2분할이 합계와 정합(원자료 보존 확인).
	if tot.PsSunsu+tot.PsEtcSunsu != tot.Electorate {
		t.Errorf("psSunsu+psEtcSunsu = %d, want electorate %d", tot.PsSunsu+tot.PsEtcSunsu, tot.Electorate)
	}
	if tot.PsTusu+tot.PsEtcTusu != tot.Votes {
		t.Errorf("psTusu+psEtcTusu = %d, want votes %d", tot.PsTusu+tot.PsEtcTusu, tot.Votes)
	}

	// 시도 행.
	if recs[1].Sido != "서울특별시" {
		t.Errorf("recs[1].Sido = %q, want 서울특별시", recs[1].Sido)
	}
}

func TestWinners(t *testing.T) {
	fixture, err := os.ReadFile("testdata/api_winners.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("pageNo") == "1" {
			w.Write(fixture)
			return
		}
		w.Write([]byte(emptyEnvelope))
	}))
	defer srv.Close()
	c := New(WithAPIBaseURL(srv.URL), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))

	recs, err := c.Winners(context.Background(), "k", "20250603", "1")
	if err != nil {
		t.Fatalf("Winners: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no winners parsed")
	}
	w := recs[0]
	if w.Name == "" || w.Party == "" {
		t.Errorf("name/party empty: %+v", w)
	}
	if w.Votes <= 0 || w.VoteRate <= 0 {
		t.Errorf("votes/voteRate not parsed: votes=%d rate=%v", w.Votes, w.VoteRate)
	}
}

func TestElections(t *testing.T) {
	fixture, err := os.ReadFile("testdata/api_elections.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("pageNo") == "1" {
			w.Write(fixture)
			return
		}
		w.Write([]byte(emptyEnvelope))
	}))
	defer srv.Close()
	c := New(WithAPIBaseURL(srv.URL), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))

	recs, err := c.Elections(context.Background(), "k")
	if err != nil {
		t.Fatalf("Elections: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("no elections parsed")
	}
	if recs[0].SgID == "" || recs[0].SgName == "" || recs[0].VoteDate == "" {
		t.Errorf("election fields empty: %+v", recs[0])
	}
}

func TestTurnoutNoData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0"?><response><header>` +
			`<resultCode>INFO-03</resultCode><resultMsg>데이터 정보가 없습니다.</resultMsg></header></response>`))
	}))
	defer srv.Close()
	c := New(WithAPIBaseURL(srv.URL), WithDelay(0), WithHTTPClient(&http.Client{Timeout: 5 * time.Second}))

	recs, err := c.Turnout(context.Background(), "k", "20260603", "3")
	if err != nil {
		t.Fatalf("INFO-03 should not be an error, got %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("got %d records, want 0 for no-data", len(recs))
	}
}

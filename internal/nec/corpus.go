package nec

// CorpusEntry is one core election-results dataset in the curated corpus.
type CorpusEntry struct {
	PublicDataPk string `json:"publicDataPk"`
	Label        string `json:"label"`
	Format       string `json:"format"` // 형식 힌트 (CSV/XLSX)
}

// CoreCorpus is the curated set of core NEC 개표결과 datasets — the essential
// election-results files a verifier wants in one fast pull. These are
// historical/cumulative datasets whose publicDataPk values are stable; new
// elections get appended here over time. This is the durable, low-churn surface
// kvote commits to maintaining (vs. fragile per-site scraping it does not).
//
// 커버리지 주의: data.go.kr 파일로 공개된 것만 담는다. 대통령선거/국회의원선거
// 데이터셋은 최신 회차만 제공되고(제18~20대 대선 등 일부 회차는 파일 미공개),
// 지방선거·구 대선은 회차별로 있다. 이 목록이 곧 kvote 가 보장하는 "핵심 코퍼스".
var CoreCorpus = []CorpusEntry{
	// 대통령선거
	{"15025528", "대통령선거 개표결과 (최신)", "CSV"},
	{"15104365", "제17대 대통령선거 개표결과", "CSV"},
	{"15133805", "제16대 대통령선거 개표결과", "CSV"},
	// 국회의원선거
	{"15025527", "국회의원선거 개표결과 (최신)", "CSV"},
	{"15144273", "비례대표국회의원선거 개표결과", "CSV"},
	// 전국동시지방선거
	{"15101509", "제8회 전국동시지방선거 개표결과", "XLSX"},
	{"15048208", "제7회 전국동시지방선거 개표결과", "XLSX"},
	{"15048207", "제6회 전국동시지방선거 개표결과", "XLSX"},
	{"15048206", "제5회 전국동시지방선거 개표결과", "XLSX"},
}

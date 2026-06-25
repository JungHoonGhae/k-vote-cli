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
var CoreCorpus = []CorpusEntry{
	{"15025528", "대통령선거 개표결과", "CSV"},
	{"15025527", "국회의원선거 개표결과", "CSV"},
	{"15144273", "비례대표국회의원선거 개표결과", "CSV"},
	{"15101509", "제8회 전국동시지방선거 개표결과", "XLSX"},
	{"15048208", "제7회 전국동시지방선거 개표결과", "XLSX"},
}

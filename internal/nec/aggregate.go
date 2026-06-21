package nec

// AggLevel selects the grouping granularity for Aggregate.
type AggLevel string

const (
	AggNone     AggLevel = "none"
	AggTown     AggLevel = "town"
	AggSgg      AggLevel = "sgg"
	AggSido     AggLevel = "sido"
	AggNational AggLevel = "national"
)

// CandidateShare is a candidate's summed votes plus the share derived from the
// group's valid votes (정의: 후보득표 / 유효투표수).
type CandidateShare struct {
	Party string  `json:"party"`
	Name  string  `json:"name"`
	Votes int     `json:"votes"`
	Share float64 `json:"share"`
}

// AggregatedRecord is one grouping of polling units rolled up to AggLevel, with
// neutral derived parameters. Candidates are summed only where comparable
// (town/sgg); at sido/national the slice is empty because districts differ.
type AggregatedRecord struct {
	Level      string           `json:"level"`
	Sido       string           `json:"sido,omitempty"`
	District   string           `json:"district,omitempty"`
	Town       string           `json:"town,omitempty"`
	VoteType   string           `json:"voteType,omitempty"`
	Electorate int              `json:"electorate"`
	Votes      int              `json:"votes"`
	Invalid    int              `json:"invalid"`
	ValidVotes int              `json:"validVotes"`
	Abstention int              `json:"abstention"`
	Turnout    float64          `json:"turnout"`
	Candidates []CandidateShare `json:"candidates,omitempty"`
}

// Aggregate rolls polling-unit records up to the requested level, optionally
// splitting each group by vote type. Group and candidate order follow first
// appearance for deterministic output. All derived values are reproducible
// arithmetic — no judgment is applied.
func Aggregate(recs []ResultRecord, level AggLevel, byVoteType bool) []AggregatedRecord {
	keepCands := level == AggTown || level == AggSgg

	type group struct {
		rec     AggregatedRecord
		candIdx map[string]int
	}
	order := []string{}
	groups := map[string]*group{}

	for _, r := range recs {
		key := groupKey(r, level, byVoteType)
		g, ok := groups[key]
		if !ok {
			g = &group{rec: newAgg(r, level, byVoteType), candIdx: map[string]int{}}
			groups[key] = g
			order = append(order, key)
		}
		g.rec.Electorate += r.Electorate
		g.rec.Votes += r.Votes
		g.rec.Invalid += r.Invalid
		g.rec.Abstention += r.Abstention
		if keepCands {
			for _, c := range r.Candidates {
				ck := c.Party + "\x1f" + c.Name
				if idx, ok := g.candIdx[ck]; ok {
					g.rec.Candidates[idx].Votes += c.Votes
				} else {
					g.candIdx[ck] = len(g.rec.Candidates)
					g.rec.Candidates = append(g.rec.Candidates, CandidateShare{Party: c.Party, Name: c.Name, Votes: c.Votes})
				}
			}
		}
	}

	out := make([]AggregatedRecord, 0, len(order))
	for _, key := range order {
		rec := groups[key].rec
		rec.ValidVotes = rec.Votes - rec.Invalid
		if rec.Electorate > 0 {
			rec.Turnout = float64(rec.Votes) / float64(rec.Electorate)
		}
		for i := range rec.Candidates {
			if rec.ValidVotes > 0 {
				rec.Candidates[i].Share = float64(rec.Candidates[i].Votes) / float64(rec.ValidVotes)
			}
		}
		out = append(out, rec)
	}
	return out
}

func groupKey(r ResultRecord, level AggLevel, byVoteType bool) string {
	var k string
	switch level {
	case AggTown:
		k = r.Sido + "\x1f" + r.District + "\x1f" + r.Town
	case AggSgg:
		k = r.Sido + "\x1f" + r.District
	case AggSido:
		k = r.Sido
	case AggNational:
		k = "전국"
	}
	if byVoteType {
		k += "\x1f" + r.VoteType
	}
	return k
}

func newAgg(r ResultRecord, level AggLevel, byVoteType bool) AggregatedRecord {
	rec := AggregatedRecord{Level: string(level)}
	switch level {
	case AggTown:
		rec.Sido, rec.District, rec.Town = r.Sido, r.District, r.Town
	case AggSgg:
		rec.Sido, rec.District = r.Sido, r.District
	case AggSido:
		rec.Sido = r.Sido
	case AggNational:
		// no spatial dimension
	}
	if byVoteType {
		rec.VoteType = r.VoteType
	}
	return rec
}

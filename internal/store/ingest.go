package store

import (
	"fmt"
	"time"

	"github.com/JungHoonGhae/k-vote-cli/internal/nec"
	"github.com/JungHoonGhae/k-vote-cli/internal/nesdc"
)

// DatasetMeta identifies a dataset for provenance and idempotent replacement.
type DatasetMeta struct {
	Source       string // "nec" | "nesdc"
	PublicDataPk string
	Name         string
	ElectionName string
}

// IngestResults replaces any existing dataset with the same (source, public_data_pk)
// and inserts the polling-unit records verbatim. All-or-nothing (transaction).
func (d *DB) IngestResults(meta DatasetMeta, recs []nec.ResultRecord) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM datasets WHERE source = ? AND public_data_pk = ?`,
		meta.Source, meta.PublicDataPk); err != nil {
		return 0, err
	}
	res, err := tx.Exec(
		`INSERT INTO datasets(source, public_data_pk, name, election_name, ingested_at, row_count)
		 VALUES(?,?,?,?,?,?)`,
		meta.Source, meta.PublicDataPk, meta.Name, meta.ElectionName,
		time.Now().UTC().Format(time.RFC3339), len(recs))
	if err != nil {
		return 0, err
	}
	dsID, _ := res.LastInsertId()

	for _, r := range recs {
		rr, err := tx.Exec(
			`INSERT INTO results(dataset_id, sido, sgg, town, booth, vote_type,
			   electorate, votes, invalid, abstention) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			dsID, r.Sido, r.District, r.Town, r.Booth, r.VoteType,
			r.Electorate, r.Votes, r.Invalid, r.Abstention)
		if err != nil {
			return 0, fmt.Errorf("insert result: %w", err)
		}
		rid, _ := rr.LastInsertId()
		for _, c := range r.Candidates {
			if _, err := tx.Exec(
				`INSERT INTO candidates(result_id, party, name, votes) VALUES(?,?,?,?)`,
				rid, c.Party, c.Name, c.Votes); err != nil {
				return 0, fmt.Errorf("insert candidate: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return dsID, nil
}

// IngestPolls is implemented in Task 4.
var _ = nesdc.PollRecord{}

package nesdc

import (
	"context"
	"fmt"
	"strings"
)

// PickTabulationAttachment picks the 집계표/통계표 (tabulation) PDF from a survey's
// attachments — the file that carries the per-question result tables (긍정/부정
// 등). Preference: name contains 집계 or 통계; else the sole non-questionnaire PDF
// (설문지/질문지 excluded); else it fails rather than guess.
func PickTabulationAttachment(atts []Attachment) (Attachment, bool) {
	for _, a := range atts {
		if strings.Contains(a.Name, "집계") || strings.Contains(a.Name, "통계") {
			return a, true
		}
	}
	var nonSurvey []Attachment
	for _, a := range atts {
		low := strings.ToLower(a.Name)
		if strings.HasSuffix(low, ".pdf") && !strings.Contains(a.Name, "설문") && !strings.Contains(a.Name, "질문") {
			nonSurvey = append(nonSurvey, a)
		}
	}
	if len(nonSurvey) == 1 {
		return nonSurvey[0], true
	}
	return Attachment{}, false
}

// Tabulation locates a survey's 집계표 PDF and downloads it into destDir. It does
// NOT parse the PDF — number reading is left to the consumer (person or AI agent).
// Returns the saved file path, the chosen attachment, and the survey Detail (for
// metadata like 조사기관명/조사일시).
func (c *Client) Tabulation(ctx context.Context, b Board, nttID, destDir string) (path string, att Attachment, d *Detail, err error) {
	d, err = c.Detail(ctx, b, nttID)
	if err != nil {
		return "", Attachment{}, nil, err
	}
	a, ok := PickTabulationAttachment(d.Attachments)
	if !ok {
		names := make([]string, len(d.Attachments))
		for i, x := range d.Attachments {
			names[i] = x.Name
		}
		return "", Attachment{}, d, fmt.Errorf("집계표 첨부를 식별하지 못함 (%d개 첨부: %s)", len(d.Attachments), strings.Join(names, ", "))
	}
	path, err = c.Download(ctx, a, destDir)
	if err != nil {
		return "", a, d, err
	}
	return path, a, d, nil
}

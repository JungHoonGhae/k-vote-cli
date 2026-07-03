package nec

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildTurnoutZip makes a minimal 성별·연령대별 투표율 xlsx and wraps it in a zip,
// matching the real layout: title row, bracket marker, 구분 header, then
// per-region blocks of (전체/남자/여자) × (선거인수/투표자수/투표율).
func buildTurnoutZip(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sh := "서울"
	f.SetSheetName("Sheet1", sh)
	set := func(cell, v string) { f.SetCellValue(sh, cell, v) }
	// row1 title, row3 marker, row4 구분 header, rows5.. data
	set("A1", "성별·연령대별 투표율(구시군별)")
	set("A3", "[표본-일반][서울특별시]")
	set("A4", "구분")
	set("D4", "합계")
	set("E4", "18세")
	set("F4", "20-24세")
	// 전체 지역, 합계 성별
	set("A5", "전체")
	set("B5", "합계")
	set("C5", "선거인수")
	set("D5", "710,801")
	set("E5", "5,910")
	set("F5", "46,137")
	set("C6", "투표자수")
	set("D6", "493,617")
	set("E6", "3,673")
	set("F6", "26,835")
	set("C7", "투표율")
	set("D7", "69.4")
	set("E7", "62.1")
	set("F7", "58.2")
	// 전체 지역, 남자
	set("B8", "남자")
	set("C8", "선거인수")
	set("D8", "339,845")
	set("C9", "투표자수")
	set("D9", "233,296")
	set("C10", "투표율")
	set("D10", "68.6")
	// 전체 지역, 여자
	set("B11", "여자")
	set("C11", "선거인수")
	set("D11", "370,956")
	set("C12", "투표자수")
	set("D12", "260,321")
	set("C13", "투표율")
	set("D13", "70.2")
	// 다음 지역 종로구, 합계만 (블록 반복 검증용)
	set("A14", "종로구")
	set("B14", "합계")
	set("C14", "선거인수")
	set("D14", "100")
	set("C15", "투표자수")
	set("D15", "60")
	set("C16", "투표율")
	set("D16", "60.0")

	var xlsxBuf bytes.Buffer
	if err := f.Write(&xlsxBuf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("02_선거일 투표/02_성별·연령대별 투표율(구시군별).xlsx")
	w.Write(xlsxBuf.Bytes())
	// 매칭 안 되는 파일도 하나 넣어 graceful-skip 검증
	w2, _ := zw.Create("03_사전투표/01_전체.xlsx")
	w2.Write([]byte("not an xlsx"))
	zw.Close()
	return zipBuf.Bytes()
}

func TestParseTurnoutAnalysis(t *testing.T) {
	recs, err := ParseTurnoutAnalysis(buildTurnoutZip(t))
	if err != nil {
		t.Fatalf("ParseTurnoutAnalysis: %v", err)
	}
	// 전체(3성별×3연령) + 종로구(1성별×3연령... 합계만) = 9 + 3 = 12
	if len(recs) != 12 {
		t.Fatalf("got %d records, want 12", len(recs))
	}
	// 전체·합계·합계(연령) 레코드 확인
	var got *TurnoutAnalysisRecord
	for i := range recs {
		r := &recs[i]
		if r.Region == "전체" && r.Gender == "합계" && r.AgeGroup == "합계" {
			got = r
			break
		}
	}
	if got == nil {
		t.Fatal("전체/합계/합계 레코드 없음")
	}
	if got.RegionLevel != "구시군" {
		t.Errorf("RegionLevel = %q, want 구시군", got.RegionLevel)
	}
	if got.Category != "표본-일반" {
		t.Errorf("Category = %q, want 표본-일반", got.Category)
	}
	if got.Sido != "서울" {
		t.Errorf("Sido = %q", got.Sido)
	}
	if got.Electorate != 710801 || got.Voters != 493617 {
		t.Errorf("electorate/voters = %d/%d, want 710801/493617", got.Electorate, got.Voters)
	}
	if got.Rate != 69.4 {
		t.Errorf("Rate = %v, want 69.4", got.Rate)
	}
	if got.Election != "" {
		t.Errorf("Election should be empty (caller fills), got %q", got.Election)
	}
}

func TestParseTurnoutAnalysisNoTarget(t *testing.T) {
	// zip with only a non-matching file → error
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("03_사전투표/01_전체.xlsx")
	w.Write([]byte("nope"))
	zw.Close()
	if _, err := ParseTurnoutAnalysis(zipBuf.Bytes()); err == nil {
		t.Fatal("expected error when no target sheet matched")
	}
}

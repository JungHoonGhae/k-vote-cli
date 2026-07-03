package nesdc

import "testing"

func TestPickTabulationAttachment(t *testing.T) {
	cases := []struct {
		name string
		atts []Attachment
		want string // 선택된 Name (또는 "" = 실패)
	}{
		{"집계표", []Attachment{{Name: "설문지_x.pdf"}, {Name: "국정현안_집계표_x.pdf"}}, "국정현안_집계표_x.pdf"},
		{"통계표", []Attachment{{Name: "aaa_통계표.pdf"}, {Name: "설문지.pdf"}}, "aaa_통계표.pdf"},
		{"비설문 단일", []Attachment{{Name: "설문지.pdf"}, {Name: "결과표.pdf"}}, "결과표.pdf"},
		{"설문지만", []Attachment{{Name: "설문지.pdf"}}, ""},
		{"모호(둘다 비설문)", []Attachment{{Name: "a.pdf"}, {Name: "b.pdf"}}, ""},
		{"질문지+결과분석", []Attachment{{Name: "전체질문지_x.pdf"}, {Name: "결과분석_x.pdf"}}, "결과분석_x.pdf"},
		{"질문지만", []Attachment{{Name: "질문지.pdf"}}, ""},
	}
	for _, tc := range cases {
		got, ok := PickTabulationAttachment(tc.atts)
		if tc.want == "" {
			if ok {
				t.Errorf("%s: 실패해야 하는데 %q 선택", tc.name, got.Name)
			}
			continue
		}
		if !ok || got.Name != tc.want {
			t.Errorf("%s: got %q(%v), want %q", tc.name, got.Name, ok, tc.want)
		}
	}
}

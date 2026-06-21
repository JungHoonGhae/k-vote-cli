package nec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FileInfo is the per-file download metadata resolved for a dataset: the values
// fileDownload.do needs to stream the actual file.
type FileInfo struct {
	PublicDataPk string `json:"publicDataPk"`
	AtchFileID   string `json:"atchFileId"`
	FileDetailSn string `json:"fileDetailSn"`
	Name         string `json:"name"`
}

// reUDDI matches the publicDataDetailPk passed to the page's download helper:
//
//	fn_fileDataDown('15025527', 'uddi:e6b1...-...', '', '1', '3')
var reUDDI = regexp.MustCompile(`fn_fileDataDown\('(\d+)',\s*'(uddi:[0-9a-fA-F-]+)'`)

// detailUDDI fetches a dataset's detail page and extracts its publicDataDetailPk
// (a uddi), which the download-resolve step requires.
func (c *Client) detailUDDI(ctx context.Context, pk string) (string, error) {
	resp, err := c.rawGet(ctx, "/data/"+pk+"/fileData.do", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	m := reUDDI.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("dataset %s: could not find download token (publicDataDetailPk)", pk)
	}
	return string(m[2]), nil
}

// downloadResolve struct mirrors the relevant fields of selectFileDataDownload.do.
type downloadResolve struct {
	AtchFileID   string `json:"atchFileId"`
	FileDetailSn any    `json:"fileDetailSn"`
	Status       bool   `json:"status"`
	Detail       struct {
		DataNm string `json:"dataNm"`
	} `json:"dataSetFileDetailInfo"`
}

// Resolve turns a publicDataPk into the concrete FileInfo needed to download,
// performing the uddi lookup and the download-metadata call.
func (c *Client) Resolve(ctx context.Context, pk string) (FileInfo, error) {
	uddi, err := c.detailUDDI(ctx, pk)
	if err != nil {
		return FileInfo{}, err
	}
	q := url.Values{}
	q.Set("publicDataPk", pk)
	q.Set("publicDataDetailPk", uddi)
	q.Set("fileDetailSn", "1")
	resp, err := c.rawGet(ctx, "/tcs/dss/selectFileDataDownload.do", q)
	if err != nil {
		return FileInfo{}, err
	}
	defer resp.Body.Close()
	var r downloadResolve
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return FileInfo{}, fmt.Errorf("dataset %s: decode download metadata: %w", pk, err)
	}
	if !r.Status || r.AtchFileID == "" {
		return FileInfo{}, fmt.Errorf("dataset %s: download not available (status=%v)", pk, r.Status)
	}
	return FileInfo{
		PublicDataPk: pk,
		AtchFileID:   r.AtchFileID,
		FileDetailSn: toStr(r.FileDetailSn),
		Name:         r.Detail.DataNm,
	}, nil
}

// Download resolves a dataset and streams its file into destDir, returning the
// written path. The server-supplied Content-Disposition filename is preferred.
func (c *Client) Download(ctx context.Context, pk, destDir string) (string, error) {
	fi, err := c.Resolve(ctx, pk)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("atchFileId", fi.AtchFileID)
	q.Set("fileDetailSn", fi.FileDetailSn)
	q.Set("dataNm", fi.Name)
	resp, err := c.rawGet(ctx, "/cmm/cmm/fileDownload.do", q)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download %s: unexpected status %s", pk, resp.Status)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(strings.ToLower(cd), "attachment") {
		return "", fmt.Errorf("download %s: not a file response", pk)
	}

	name := filenameFromCD(cd)
	if name == "" {
		name = fi.Name
	}
	name = sanitizeFilename(name)
	if name == "" {
		name = pk
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(destDir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return "1"
	default:
		return fmt.Sprintf("%v", t)
	}
}

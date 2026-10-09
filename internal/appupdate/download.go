package appupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func download(ctx context.Context, c *http.Client, a Asset, path string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	if total <= 0 {
		total = a.Size
	}
	h := sha256.New()
	var body io.Reader = resp.Body
	if a.Size > 0 {
		body = io.LimitReader(body, a.Size+1)
	}
	if progress != nil {
		body = &counter{r: body, total: max(total, 0), report: progress}
	}
	_, err = io.Copy(io.MultiWriter(f, h), body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), a.SHA256) {
		err = errors.New("the download does not match its checksum")
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// counter reports bytes as they are read.
type counter struct {
	r      io.Reader
	done   int64
	total  int64
	report func(done, total int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	c.report(c.done, c.total)
	return n, err
}

package virtualbox

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/iodasolutions/xbee-common/cmd"
	"github.com/iodasolutions/xbee-common/log2"
	"github.com/iodasolutions/xbee-common/newfs"
	"io"
	"net/http"
	"os"
	"time"
)

const bytesToMegaBytes = 1048576.0

func DownloadIfNotCached(ctx context.Context, rawUrl string) (newfs.File, *cmd.XbeeError) {
	f := newfs.CachedFileForUrl(rawUrl)
	if !f.Exists() {
		log2.Debugf("file %s do not exist in cache", f)
		err := DoDownload(ctx, rawUrl)
		return f, err
	} else {
		log2.Debugf("Found %s in xbee cache\n", f.Base())
	}
	return f, nil
}
func DoDownload(ctx context.Context, rawUrl string) *cmd.XbeeError {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}
	// Effectuer la requête GET
	resp, err := client.Get(rawUrl)
	if err != nil {
		return cmd.Error("Failed invoke http GET on url %s : %v\n", rawUrl, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return cmd.Error("Server responded with status code = %d for url %s", resp.StatusCode, rawUrl)
	}
	pt := NewPathThru(resp.Body, resp.ContentLength)
	f := newfs.CachedFileForUrl(rawUrl)
	return pt.DownloadTo(f)
}

type PassThru struct {
	io.Reader
	curr  int64
	total int64
	start time.Time
}

func NewPathThru(r io.Reader, length int64) *PassThru {
	return &PassThru{
		Reader: r,
		total:  length,
		start:  time.Now(),
	}
}

func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.curr += int64(n)

	// last read will have EOF err
	if err == nil || (err == io.EOF && n > 0) {
		pt.printProgress(pt.curr, pt.total)
	}
	return n, err
}

func (pt *PassThru) printProgress(curr, total int64) {
	width := 40.0
	output := ""
	threshold := (float64(curr) / float64(total)) * width
	for i := 0.0; i < width; i++ {
		if i < threshold {
			if output == "" {
				output = ">"
			} else {
				output = "=" + output
			}
		} else {
			output += " "
		}
	}
	perc := (float64(curr) / float64(total)) * 100
	duree := time.Now().Sub(pt.start)
	message := fmt.Sprintf("\r%3.0f%%[%s] %.1fMB %.2fMB/s eta %v", perc, output, float64(curr)/bytesToMegaBytes, float64(curr)/bytesToMegaBytes/duree.Seconds(), duree.Round(time.Second))
	fmt.Print(message)
}

func (pt *PassThru) DownloadTo(f newfs.File) *cmd.XbeeError {
	tmpFile := newfs.NewFile(f.String() + ".tmp")
	size := tmpFile.FillFromReader(pt)
	fmt.Print("\n")
	if err := os.Rename(tmpFile.String(), f.String()); err != nil {
		return cmd.Error("cannot rename %s to %s", tmpFile, f)
	}
	log2.Infof("Resource %s Transferred. (%.1f MB)\n", f.Base(), float64(size)/bytesToMegaBytes)
	return nil
}

package tent

import (
	"archive/tar"
	"io"
	"log/slog"
	"mansionTent/share"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/xi2/xz"
)

type launcher struct {
	sitter   *sitter
	s3       *s3.S3
	s3folder url.URL
}

func RunLauncher() {
	NewLauncher().Run()
}

func NewLauncher() *launcher {
	region := os.Getenv("AWS_REGION_S3")
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	aws := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(region)},
	}))
	t := &launcher{s3: s3.New(aws)}
	t.sitter = NewSitter(NewHooks(t))

	parsed, err := url.Parse(os.Getenv("S3_FOLDER_URL"))
	if err != nil {
		panic(err)
	}
	parsed.Path = strings.Trim(parsed.Path, "/")
	t.s3folder = *parsed
	t.s3folder.Path = strings.Trim(t.s3folder.Path, "/")
	return t
}

func (t *launcher) Run() {
	slog.Info("Starting launcher")
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go t.downloadGame(&waitGroup)
	go t.downloadState(&waitGroup)
	waitGroup.Wait()
	os.Chdir("factorio")
	t.sitter.Run()
}

func (t *launcher) downloadGame(wg *sync.WaitGroup) {
	defer wg.Done()
	// check if we need to do this
	_, err := os.Stat("factorio/bin/x64/factorio")
	if err == nil {
		slog.Info("Game already downloaded")
		return
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	// download
	timer := share.NewPerfTimer()
	url := "https://www.factorio.com/get-download/latest/headless/linux64"
	slog.Info("Downloading game from", "url", url)
	download, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer download.Body.Close()
	decompress, err := xz.NewReader(download.Body, 0)
	if err != nil {
		panic(err)
	}
	// we'll unpack everything as we download it
	unpack := tar.NewReader(decompress)
	for t.unpackOneFile(unpack) {
	}
	slog.Info("Downloaded game files", "elapsed", timer)
}

func (t *launcher) unpackOneFile(unpack *tar.Reader) bool {
	header, err := unpack.Next()
	if err == io.EOF {
		return false // end of tar archive
	} else if err != nil {
		panic(err)
	}
	slog.Debug("Unpacking", "file", header.Name)
	mode := header.FileInfo().Mode()
	switch header.Typeflag {
	case tar.TypeDir:
		err := os.MkdirAll(header.Name, mode)
		if err != nil {
			panic(err)
		}
	case tar.TypeReg:
		err := os.MkdirAll(filepath.Dir(header.Name), mode|mode>>2&0o111)
		if err != nil {
			panic(err)
		}
		file, err := os.OpenFile(header.Name, os.O_CREATE|os.O_WRONLY, mode)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		_, err = io.Copy(file, unpack)
		if err != nil {
			panic(err)
		}
	}
	return true // continue unpacking
}

func (t *launcher) downloadState(wg *sync.WaitGroup) {
	defer wg.Done()
	// check if we need to do this
	_, err := os.Stat("factorio/saves")
	if err == nil {
		slog.Info("State files already downloaded")
		return
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	// start downloaders
	timer := share.NewPerfTimer()
	queue := make(chan *string, 5)
	for i := 0; i < cap(queue); i++ {
		go func() {
			for key := range queue {
				if key == nil {
					return
				}
				t.downloadOneFile(key)
			}
		}()
	}
	// enumerate files from s3
	slog.Info("Downloading save files from", "s3", t.s3folder.String())
	request := &s3.ListObjectsInput{
		Bucket: aws.String(t.s3folder.Host),
		Prefix: aws.String(t.s3folder.Path + "/"),
	}
	for nextPage := aws.String("first"); nextPage != nil; nextPage = request.Marker {
		response, err := t.s3.ListObjects(request)
		if err != nil {
			panic(err)
		}
		for _, object := range response.Contents {
			// dunno if this can be nil, but if it ever happens there will be a deadlock
			if object.Key != nil {
				queue <- object.Key
			}
		}
		request.Marker = response.NextMarker
	}
	// wait for downloaders to finish
	for i := 0; i < cap(queue); i++ {
		queue <- nil
	}
	slog.Info("Downloaded save and config/mod files", "elapsed", timer)
}

func (t *launcher) downloadOneFile(key *string) {
	destPath := "factorio/" + strings.TrimPrefix(*key, t.s3folder.Path+"/")
	slog.Debug("Downloading", "file", *key, "to", destPath)
	// download the source file
	response, err := t.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(t.s3folder.Host),
		Key:    key,
	})
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	// create the destination file
	err = os.MkdirAll(filepath.Dir(destPath), 0o755)
	if err != nil {
		panic(err)
	}
	file, err := os.Create(destPath)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	// copy the file
	_, err = io.Copy(file, response.Body)
	if err != nil {
		panic(err)
	}
}

func (t *launcher) uploadSave() {
	// get the most recent save
	var mostRecent string
	var mostRecentTime time.Time
	filepath.Walk("saves", func(path string, info os.FileInfo, err error) error {
		slog.Debug("Checking", "file", path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".zip") {
			modTime := info.ModTime()
			if mostRecentTime.Before(modTime) {
				mostRecentTime = info.ModTime()
				mostRecent = path
			}
		}
		return nil
	})
	if mostRecent == "" {
		slog.Warn("No save files found")
		return
	}
	// upload the save
	timer := share.NewPerfTimer()
	slog.Info("Uploading save", "file", mostRecent)
	file, err := os.Open(mostRecent)
	if err != nil {
		slog.Error("Error opening file", "err", err)
		return
	}
	defer file.Close()
	_, err = t.s3.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(t.s3folder.Host),
		Key:    aws.String(t.s3folder.Path + "/" + mostRecent),
		Body:   file,
	})
	if err != nil {
		slog.Error("Error uploading file", "err", err)
	}
	slog.Info("Uploaded save", "file", mostRecent, "elapsed", timer)
}

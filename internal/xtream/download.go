// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package xtream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bandwidth struct {
	mu             sync.Mutex
	next           time.Time
	bytesPerSecond float64
}

func (b *bandwidth) wait(ctx context.Context, bytes int) error {
	if b.bytesPerSecond == 0 || bytes == 0 {
		return ctx.Err()
	}
	b.mu.Lock()
	now := time.Now()
	if b.next.Before(now) {
		b.next = now
	}
	b.next = b.next.Add(time.Duration(float64(time.Second) * float64(bytes) / b.bytesPerSecond))
	delay := time.Until(b.next)
	b.mu.Unlock()
	return waitFor(ctx, delay)
}

func waitFor(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// DownloadBatch runs episode transfers within one provider's concurrency and bandwidth limits.
func (p *Providers) DownloadBatch(ctx context.Context, name string, episodes []Episode, transfer func(Episode, func(*os.File, func() error) error)) error {
	selected, err := p.selectProviders(name)
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("download requires an explicit provider")
	}
	entry := selected[0]
	parallel := 1
	if entry.settings.Concurrent != nil {
		parallel = *entry.settings.Concurrent
	}
	limiter := &bandwidth{bytesPerSecond: float64(entry.settings.BandwidthKBPS) * 1000}
	jobs := make(chan Episode)
	var workers sync.WaitGroup
	for range min(parallel, len(episodes)) {
		workers.Go(func() {
			for episode := range jobs {
				transfer(episode, func(file *os.File, validate func() error) error {
					return p.download(ctx, entry, episode, limiter, file, validate)
				})
			}
		})
	}
	defer workers.Wait()
	defer close(jobs)
	for _, episode := range episodes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case jobs <- episode:
		}
	}
	return nil
}

func (p *Providers) download(ctx context.Context, entry provider, episode Episode, limiter *bandwidth, file *os.File, validate func() error) error {
	if !validID(episode.ID) || !extensionPattern.MatchString(episode.Extension) {
		return errors.New("invalid Xtream episode ID or extension")
	}
	target := entry.url.JoinPath("series")
	// Escape each path segment separately because provider credentials may contain delimiters.
	target.RawPath = strings.TrimRight(target.EscapedPath(), "/") + "/" + url.PathEscape(entry.settings.Username) + "/" + url.PathEscape(entry.settings.Password) + "/" + episode.ID + "." + episode.Extension
	decoded, err := url.PathUnescape(target.RawPath)
	if err != nil {
		return errors.New("invalid Xtream download path")
	}
	target.Path = decoded
	client := &http.Client{Transport: p.client.Transport, CheckRedirect: func(req *http.Request, previous []*http.Request) error {
		if len(previous) >= 10 || req.URL.User != nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
			return errors.New("invalid download redirect")
		}
		req.Header.Del("Referer")
		if req.URL.Scheme != target.Scheme || !strings.EqualFold(req.URL.Host, target.Host) {
			for header := range entry.settings.Headers {
				req.Header.Del(header)
			}
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			req.Header.Set("User-Agent", p.agent)
		}
		return nil
	}}
	retries := 2
	if entry.settings.Retries != nil {
		retries = *entry.settings.Retries
	}
	delay := entry.retryDelay
	for attempt := 0; ; attempt++ {
		retry, override, err := p.transfer(ctx, client, entry, target.String(), limiter, file)
		if err == nil {
			err = validate()
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := file.Truncate(0); err != nil {
				return err
			}
			retry = true
		}
		if !retry || attempt >= retries {
			return err
		}
		if override < 0 {
			override = delay
		}
		if err := waitFor(ctx, override); err != nil {
			return err
		}
		if delay <= time.Duration(1<<62-1) {
			delay *= 2
		}
	}
}

func (p *Providers) transfer(ctx context.Context, client *http.Client, entry provider, target string, limiter *bandwidth, file *os.File) (bool, time.Duration, error) {
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return false, 0, err
	}
	requestContext, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, "GET", target, nil)
	if err != nil {
		return false, 0, errors.New("could not construct Xtream download request")
	}
	req.Header.Set("User-Agent", p.agent)
	for key, value := range entry.settings.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	} else {
		req.Header.Del("Range")
	}
	timer := time.AfterFunc(p.client.Timeout, cancel)
	response, err := client.Do(req)
	timer.Stop()
	if err != nil {
		if ctx.Err() != nil {
			return false, 0, ctx.Err()
		}
		return true, -1, errors.New("Xtream download connection failed")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusTooManyRequests:
		return true, retryAfter(response.Header.Get("Retry-After")), errors.New("Xtream returned HTTP 429")
	case http.StatusRequestedRangeNotSatisfiable:
		if offset > 0 {
			if err := file.Truncate(0); err != nil {
				return false, 0, err
			}
			return true, 0, errors.New("Xtream rejected resumed range")
		}
		return false, 0, errors.New("Xtream returned HTTP 416")
	case http.StatusOK:
		if offset > 0 {
			if err := file.Truncate(0); err != nil {
				return false, 0, err
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return false, 0, err
			}
		}
	case http.StatusPartialContent:
		if !validRange(response.Header.Get("Content-Range"), offset, response.ContentLength) {
			if err := file.Truncate(0); err != nil {
				return false, 0, err
			}
			return true, 0, errors.New("invalid Xtream Content-Range")
		}
	default:
		return false, 0, fmt.Errorf("Xtream returned HTTP %d", response.StatusCode)
	}
	buffer := make([]byte, 32<<10)
	for {
		timer = time.AfterFunc(p.client.Timeout, cancel)
		size, readErr := response.Body.Read(buffer)
		timer.Stop()
		if size > 0 {
			if err := limiter.wait(ctx, size); err != nil {
				return false, 0, err
			}
			if _, err := file.Write(buffer[:size]); err != nil {
				return false, 0, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return false, 0, nil
		}
		if readErr != nil {
			return true, -1, errors.New("Xtream download response was interrupted")
		}
	}
}

func validRange(header string, offset, length int64) bool {
	startAndEnd, total, ok := strings.Cut(strings.TrimPrefix(header, "bytes "), "/")
	start, end, ok2 := strings.Cut(startAndEnd, "-")
	first, firstErr := strconv.ParseInt(start, 10, 64)
	last, lastErr := strconv.ParseInt(end, 10, 64)
	size, sizeErr := strconv.ParseInt(total, 10, 64)
	return strings.HasPrefix(header, "bytes ") && ok && ok2 && firstErr == nil && lastErr == nil && sizeErr == nil &&
		first == offset && first <= last && last == size-1 && (length < 0 || length == size-first)
}

func retryAfter(raw string) time.Duration {
	if seconds, err := strconv.ParseInt(raw, 10, 32); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(raw); err == nil {
		return max(0, time.Until(date))
	}
	return -1
}

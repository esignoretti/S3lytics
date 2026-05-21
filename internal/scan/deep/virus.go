package deep

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

type VirusScanConfig struct {
	ClamdSocket string
	Extensions  []string
	MaxSize     int64
	MaxCount    int
	LastSince   time.Time
}

func ScanObjectsForVirus(ctx context.Context, client s3.S3Client, bucket string, config VirusScanConfig) (*store.VirusResult, error) {
	result := &store.VirusResult{
		Status:   "completed",
		Scanned:  0,
		Infected: []string{},
		Errors:   []string{},
	}

	extSet := make(map[string]bool)
	for _, ext := range config.Extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extSet[strings.ToLower(ext)] = true
	}

	var continuationToken *string
	scannedCount := 0

	for {
		page, err := client.ListObjectsPage(ctx, bucket, continuationToken)
		if err != nil {
			break
		}

		for _, obj := range page.Objects {
			if config.MaxCount > 0 && scannedCount >= config.MaxCount {
				return result, nil
			}

			if len(extSet) > 0 {
				ext := strings.ToLower(extractExtension(obj.Key))
				if !extSet[ext] {
					continue
				}
			}

			if config.MaxSize > 0 && obj.Size > config.MaxSize {
				continue
			}

			if !config.LastSince.IsZero() && obj.LastModified.Before(config.LastSince) {
				continue
			}

			data, err := client.GetObject(ctx, bucket, obj.Key, nil)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", obj.Key, err))
				continue
			}

			infected, err := scanWithClamd(data, config.ClamdSocket)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", obj.Key, err))
				continue
			}

			scannedCount++
			if infected != "" {
				result.Infected = append(result.Infected, fmt.Sprintf("%s: %s", obj.Key, infected))
			}
		}

		if !page.IsTruncated {
			break
		}
		continuationToken = page.ContinuationToken
	}

	result.Scanned = scannedCount
	if len(result.Infected) > 0 {
		result.Status = "completed_with_infections"
	}

	return result, nil
}

func scanWithClamd(data []byte, socketAddr string) (string, error) {
	var conn net.Conn
	var err error

	if strings.HasPrefix(socketAddr, "tcp://") {
		conn, err = net.DialTimeout("tcp", socketAddr[6:], 30*time.Second)
	} else {
		conn, err = net.DialTimeout("unix", socketAddr, 30*time.Second)
	}
	if err != nil {
		return "", fmt.Errorf("clamd connect: %w", err)
	}
	defer conn.Close()

	cmd := []byte("zINSTREAM\x00")
	if _, err := conn.Write(cmd); err != nil {
		return "", fmt.Errorf("send instream: %w", err)
	}

	chunkSize := 1024 * 64
	reader := bytes.NewReader(data)
	buf := make([]byte, chunkSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			lenBytes := []byte{
				byte(n >> 24),
				byte(n >> 16),
				byte(n >> 8),
				byte(n),
			}
			if _, err := conn.Write(lenBytes); err != nil {
				return "", fmt.Errorf("send length: %w", err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("send chunk: %w", err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read chunk: %w", err)
		}
	}

	conn.Write([]byte{0, 0, 0, 0})

	respBuf := make([]byte, 4096)
	n, err := conn.Read(respBuf)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	response := string(respBuf[:n])
	if strings.Contains(response, "FOUND") {
		parts := strings.Split(strings.TrimSpace(response), ":")
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[len(parts)-1]), nil
		}
		return "unknown virus", nil
	}

	return "", nil
}

func extractExtension(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '.' {
			return key[i:]
		}
		if key[i] == '/' {
			break
		}
	}
	return "(no extension)"
}

package scan

import (
	"math"
	"sort"
	"time"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

type statsAggregator struct {
	totalObjects int64
	totalSize    int64
	sizes        []int64
	maxSize      int64
	emptyObjects int64

	typeMap    map[string]*typeAccum
	ageBuckets map[string]*ageAccum
	storageMap map[string]*storageAccum
	prefixMap  map[string]*prefixAccum

}

type typeAccum struct {
	count int64
	size  int64
}

type ageAccum struct {
	count int64
	size  int64
}

type storageAccum struct {
	count int64
	size  int64
}

type prefixAccum struct {
	count int64
	size  int64
}

func newStatsAggregator() *statsAggregator {
	return &statsAggregator{
		typeMap:    make(map[string]*typeAccum),
		ageBuckets: make(map[string]*ageAccum),
		storageMap: make(map[string]*storageAccum),
		prefixMap:  make(map[string]*prefixAccum),
	}
}

func (a *statsAggregator) addObject(obj s3.ObjectInfo) {
	a.totalObjects++
	a.totalSize += obj.Size
	a.sizes = append(a.sizes, obj.Size)

	if obj.Size > a.maxSize {
		a.maxSize = obj.Size
	}
	if obj.Size == 0 {
		a.emptyObjects++
	}

	ext := extractExtension(obj.Key)
	if _, ok := a.typeMap[ext]; !ok {
		a.typeMap[ext] = &typeAccum{}
	}
	a.typeMap[ext].count++
	a.typeMap[ext].size += obj.Size

	ageLabel := ageBucketLabel(obj.LastModified)
	if _, ok := a.ageBuckets[ageLabel]; !ok {
		a.ageBuckets[ageLabel] = &ageAccum{}
	}
	a.ageBuckets[ageLabel].count++
	a.ageBuckets[ageLabel].size += obj.Size

	sc := obj.StorageClass
	if sc == "" {
		sc = "STANDARD"
	}
	if _, ok := a.storageMap[sc]; !ok {
		a.storageMap[sc] = &storageAccum{}
	}
	a.storageMap[sc].count++
	a.storageMap[sc].size += obj.Size

	prefix := topLevelPrefix(obj.Key)
	if _, ok := a.prefixMap[prefix]; !ok {
		a.prefixMap[prefix] = &prefixAccum{}
	}
	a.prefixMap[prefix].count++
	a.prefixMap[prefix].size += obj.Size
}

func (a *statsAggregator) buildSummary() store.ScanSummary {
	sort.Slice(a.sizes, func(i, j int) bool { return a.sizes[i] < a.sizes[j] })

	var median int64
	if len(a.sizes) > 0 {
		mid := len(a.sizes) / 2
		if len(a.sizes)%2 == 0 {
			median = (a.sizes[mid-1] + a.sizes[mid]) / 2
		} else {
			median = a.sizes[mid]
		}
	}

	var avg float64
	if a.totalObjects > 0 {
		avg = float64(a.totalSize) / float64(a.totalObjects)
	}

	return store.ScanSummary{
		TotalObjects: a.totalObjects,
		TotalSize:    a.totalSize,
		AvgSize:      math.Round(avg*100) / 100,
		MedianSize:   median,
		MaxSize:      a.maxSize,
		EmptyObjects: a.emptyObjects,
	}
}

func (a *statsAggregator) buildTypes() []store.TypeBreakdown {
	var result []store.TypeBreakdown
	for ext, acc := range a.typeMap {
		pct := 0.0
		if a.totalObjects > 0 {
			pct = math.Round(float64(acc.count)/float64(a.totalObjects)*10000) / 100
		}
		result = append(result, store.TypeBreakdown{
			Ext:       ext,
			Count:     acc.count,
			TotalSize: acc.size,
			Pct:       pct,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (a *statsAggregator) buildAges() []store.AgeBucket {
	var result []store.AgeBucket
	for label, acc := range a.ageBuckets {
		result = append(result, store.AgeBucket{
			Label: label,
			Count: acc.count,
			Size:  acc.size,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (a *statsAggregator) buildStorage() []store.StorageBreakdown {
	var result []store.StorageBreakdown
	for class, acc := range a.storageMap {
		result = append(result, store.StorageBreakdown{
			Class: class,
			Count: acc.count,
			Size:  acc.size,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
}

func (a *statsAggregator) buildPrefixes() []store.PrefixBreakdown {
	var result []store.PrefixBreakdown
	for prefix, acc := range a.prefixMap {
		result = append(result, store.PrefixBreakdown{
			Prefix: prefix,
			Count:  acc.count,
			Size:   acc.size,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return result
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

func topLevelPrefix(key string) string {
	for i, c := range key {
		if c == '/' {
			return key[:i]
		}
	}
	return key
}

func ageBucketLabel(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "<24h"
	case d < 7*24*time.Hour:
		return "<7d"
	case d < 30*24*time.Hour:
		return "<30d"
	case d < 90*24*time.Hour:
		return "<90d"
	case d < 365*24*time.Hour:
		return "<1y"
	default:
		return ">1y"
	}
}

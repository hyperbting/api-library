package meta

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi"
	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/result"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
)

const (
	ESIdxAttestationRecord          = "meta-attestation-record"
	ESIdxAttestationDeviceBanRecord = "meta-attestation-device-ban-record"
)

var (
	arSize       = 100
	arSortTsDesc = []types.SortCombinations{
		types.SortOptions{
			SortOptions: map[string]types.FieldSort{
				"timestamp": {Order: &sortorder.Desc},
			},
		},
	}
)

type MetaESRepository interface {
	CreateMetaAttestationRecord(ctx context.Context, a *ESAttestationRecord) error
	CreateAttestationDeviceBanRecord(ctx context.Context, a *ESDeviceBanRecord) error

	FindAttestationRecords(ctx context.Context, criteria AttestationCriteria, size *int, sorts []types.SortCombinations) ([]ESAttestationRecord, error)
	FindAttestationDeviceBanRecords(ctx context.Context, criteria AttestationDeviceBanCriteria, size *int, sorts []types.SortCombinations) ([]ESDeviceBanRecord, error)
}

type metaESRepositoryImpl struct {
	client *typedapi.API
}

func NewMetaESRepository(client *typedapi.API) MetaESRepository {
	return &metaESRepositoryImpl{client: client}
}

func (r *metaESRepositoryImpl) CreateMetaAttestationRecord(ctx context.Context, a *ESAttestationRecord) error {
	res, err := r.client.Index(ESIdxAttestationRecord).Request(a).Do(ctx)
	if err != nil {
		return fmt.Errorf("es index execution failed: %w", err)
	}

	// Evaluate ES index result inside the repo layer
	if res.Result != result.Created && res.Result != result.Updated {
		return fmt.Errorf("unexpected es index result status: %s", res.Result)
	}

	return nil
}

type AttestationCriteria struct {
	AppScopedID string
	AppSource   string // Leave empty to search across ALL apps
}

func (c *AttestationCriteria) FormQueries() []types.Query {
	var filters []types.Query

	// Use Term query for exact ID / Tag matches
	if c.AppSource != "" {
		filters = append(filters, types.Query{
			Term: map[string]types.TermQuery{
				"app_source.keyword": {Value: c.AppSource},
			},
		})
	}

	if c.AppScopedID != "" {
		filters = append(filters, types.Query{
			Term: map[string]types.TermQuery{
				"app_scoped_id.keyword": {Value: c.AppScopedID},
			},
		})
	}

	return filters
}

func (c *AttestationCriteria) FormSearchRequest(size *int, sorts []types.SortCombinations) search.Request {
	filters := c.FormQueries()

	q := types.Query{}
	if len(filters) > 0 {
		q.Bool = &types.BoolQuery{Filter: filters}
	} else {
		q.MatchAll = &types.MatchAllQuery{}
	}

	sr := search.Request{Query: &q}

	// Apply size safeguards
	if size == nil || *size <= 0 {
		sr.Size = &arSize
	} else if *size > arSize {
		sr.Size = &arSize
	} else {
		sr.Size = size
	}

	// Apply sorts if provided
	if len(sorts) > 0 {
		sr.Sort = sorts
	} else {
		sr.Sort = arSortTsDesc
	}

	return sr
}

func (r *metaESRepositoryImpl) FindAttestationRecords(ctx context.Context, criteria AttestationCriteria, size *int, sorts []types.SortCombinations) (ars []ESAttestationRecord, err error) {
	searchReq := criteria.FormSearchRequest(size, sorts)

	// Execute
	var res *search.Response
	if res, err = r.client.Search().Index(ESIdxAttestationRecord).Request(&searchReq).Do(ctx); err != nil {
		return []ESAttestationRecord{}, err
	}

	// Parse
	ars = make([]ESAttestationRecord, 0, len(res.Hits.Hits))
	for _, d := range res.Hits.Hits {
		var ar ESAttestationRecord
		if errJson := json.Unmarshal(d.Source_, &ar); errJson == nil {
			ars = append(ars, ar)
		}
	}

	return ars, nil
}

func (r *metaESRepositoryImpl) CreateAttestationDeviceBanRecord(ctx context.Context, a *ESDeviceBanRecord) error {
	res, err := r.client.Index(ESIdxAttestationDeviceBanRecord).Request(a).Do(ctx)
	if err != nil {
		return fmt.Errorf("es index execution failed: %w", err)
	}

	// Evaluate ES index result inside the repo layer
	if res.Result != result.Created && res.Result != result.Updated {
		return fmt.Errorf("unexpected es index result status: %s", res.Result)
	}

	return nil
}

type AttestationDeviceBanCriteria struct {
	AttestationCriteria
}

func (r *metaESRepositoryImpl) FindAttestationDeviceBanRecords(ctx context.Context, criteria AttestationDeviceBanCriteria, size *int, sorts []types.SortCombinations) (brs []ESDeviceBanRecord, err error) {
	searchReq := criteria.FormSearchRequest(size, sorts)

	// Execute
	var res *search.Response
	if res, err = r.client.Search().Index(ESIdxAttestationDeviceBanRecord).Request(&searchReq).Do(ctx); err != nil {
		return []ESDeviceBanRecord{}, err
	}

	// Parse
	brs = make([]ESDeviceBanRecord, 0, len(res.Hits.Hits))
	for _, d := range res.Hits.Hits {
		var br ESDeviceBanRecord
		if errJson := json.Unmarshal(d.Source_, &br); errJson == nil {
			brs = append(brs, br)
		}
	}

	return brs, nil
}

package meta

import (
	"api-library/pkg/platform_helper/meta"
	"context"
	"time"
)

type MetaAttestationService interface {
	SaveAttestationRecord(ctx context.Context, appScopedID string, claims *AttestationClaims) error
	SaveAttestationDeviceBanRecord(ctx context.Context, b *ESDeviceBanRecord) error

	LoadAttestationRecord(ctx context.Context, appScopedID string) ([]ESAttestationRecord, error)
}

type metaAttestationServiceImpl struct {
	attestClient meta.MetaAttestationClient
	metaClient   meta.MetaApiClient
	esRepo       MetaESRepository
}

func NewMetaAttestationService(attestClient meta.MetaAttestationClient, metaClient meta.MetaApiClient, esRepo MetaESRepository) MetaAttestationService {
	return &metaAttestationServiceImpl{
		attestClient: attestClient,
		metaClient:   metaClient,
		esRepo:       esRepo,
	}
}

func (s *metaAttestationServiceImpl) SaveAttestationRecord(ctx context.Context, appScopedID string, claims *AttestationClaims) error {
	rd := ESAttestationRecord{
		AppScopedID: appScopedID,
		AppSource:   "", // TODO:
		Timestamp:   time.Now().UTC(),
		Claims:      *claims,
	}
	return s.esRepo.CreateMetaAttestationRecord(ctx, &rd)
}

func (s *metaAttestationServiceImpl) SaveAttestationDeviceBanRecord(ctx context.Context, b *ESDeviceBanRecord) error {
	return s.esRepo.CreateAttestationDeviceBanRecord(ctx, b)
}

func (s *metaAttestationServiceImpl) LoadAttestationRecord(ctx context.Context, appScopedID string) ([]ESAttestationRecord, error) {
	return s.esRepo.FindAttestationRecords(ctx, AttestationCriteria{
		AppScopedID: appScopedID,
		AppSource:   "", // TODO:
	},
		nil,
		nil,
	)
}

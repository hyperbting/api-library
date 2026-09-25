package meta

import (
	"api-library/pkg/platform_helper/meta"
	"context"
	"time"
)

type MetaAttestationService interface {
	VerifyAndRecordAttestation(ctx context.Context, appScopedID string, claims *AttestationClaims) error
	BanDevice(ctx context.Context, b *ESDeviceBanRecord) error
	GetAttestationHistory(ctx context.Context, appScopedID string) ([]ESAttestationRecord, error)
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

func (s *metaAttestationServiceImpl) VerifyAndRecordAttestation(ctx context.Context, appScopedID string, claims *AttestationClaims) error {
	rd := ESAttestationRecord{
		AppScopedID: appScopedID,
		AppSource:   "", // TODO:
		Timestamp:   time.Now().UTC(),
		Claims:      *claims,
	}
	return s.esRepo.CreateMetaAttestationRecord(ctx, &rd)
}

func (s *metaAttestationServiceImpl) BanDevice(ctx context.Context, b *ESDeviceBanRecord) error {
	return s.esRepo.CreateAttestationDeviceBanRecord(ctx, b)
}

func (s *metaAttestationServiceImpl) GetAttestationHistory(ctx context.Context, appScopedID string) ([]ESAttestationRecord, error) {
	return s.esRepo.FindAttestationRecords(ctx, AttestationCriteria{
		AppScopedID: appScopedID,
		AppSource:   "", // TODO:
	},
		nil,
		nil,
	)
}

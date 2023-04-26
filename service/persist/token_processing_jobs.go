package persist

import (
	"database/sql/driver"
	"encoding/json"
)

type ProcessingCause string

const (
	ProcessingCauseSync    ProcessingCause = "sync"
	ProcessingCauseRefresh ProcessingCause = "refresh"
)

func (p ProcessingCause) String() string {
	return string(p)
}

func (p ProcessingCause) Value() (driver.Value, error) {
	if p == "" {
		panic("empty ProcessingCause")
	}
	return p.String(), nil
}

func (p *ProcessingCause) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	*p = ProcessingCause(value.(string))
	return nil
}

type TokenProperties struct {
	HasPrimaryMedia bool `json:"has_primary_media"`
	HasThumbnail    bool `json:"has_thumbnail"`
	HasLiveRender   bool `json:"has_live_render"`
	HasDimensions   bool `json:"has_dimensions"`
	HasMetadata     bool `json:"has_metadata"`
	HasName         bool `json:"has_name"`
	HasDescription  bool `json:"has_description"`
}

type PipelineStepStatus string

const (
	PipelineStepStatusNotRun  PipelineStepStatus = "not_run"
	PipelineStepStatusStarted PipelineStepStatus = "started"
	PipelineStepStatusSuccess PipelineStepStatus = "success"
	PipelineStepStatusError   PipelineStepStatus = "error"
)

func (p PipelineStepStatus) String() string {
	return string(p)
}

func (p PipelineStepStatus) Value() (driver.Value, error) {
	if p == "" {
		return PipelineStepStatusNotRun.String(), nil
	}
	return p.String(), nil
}

func (p *PipelineStepStatus) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	*p = PipelineStepStatus(value.(string))
	return nil
}

func (p PipelineStepStatus) MarshalJSON() ([]byte, error) {
	if p == "" {
		return json.Marshal(PipelineStepStatusNotRun.String())
	}
	return json.Marshal(p.String())
}

type PipelineMetadata struct {
	MetadataRetrieval            PipelineStepStatus `json:"metadata_retrieval,omitempty"`
	TokenInfoRetrieval           PipelineStepStatus `json:"token_info_retrieval,omitempty"`
	MediaURLsRetrieval           PipelineStepStatus `json:"media_urls_retrieval,omitempty"`
	ContentHeaderValueRetrieval  PipelineStepStatus `json:"content_header_value_retrieval,omitempty"`
	ReaderRetrieval              PipelineStepStatus `json:"reader_retrieval,omitempty"`
	OpenseaFallback              PipelineStepStatus `json:"opensea_fallback,omitempty"`
	DetermineMediaTypeWithReader PipelineStepStatus `json:"determine_media_type_with_reader,omitempty"`
	AnimationGzip                PipelineStepStatus `json:"animation_gzip,omitempty"`
	StoreGCP                     PipelineStepStatus `json:"store_gcp,omitempty"`
	ThumbnailGCP                 PipelineStepStatus `json:"thumbnail_gcp,omitempty"`
	LiveRenderGCP                PipelineStepStatus `json:"live_render_gcp,omitempty"`
	RawMediaDetermination        PipelineStepStatus `json:"raw_media_determination,omitempty"`
	NothingCachedWithErrors      PipelineStepStatus `json:"nothing_cached_errors,omitempty"`
	NothingCachedWithoutErrors   PipelineStepStatus `json:"nothing_cached_no_errors,omitempty"`
	CreateMediaFromCachedObjects PipelineStepStatus `json:"create_media_from_cached_objects,omitempty"`
	SetUnknownMediaType          PipelineStepStatus `json:"set_default_media_type,omitempty"`
	MediaResultComparison        PipelineStepStatus `json:"media_result_comparison,omitempty"`
	UpdateTokenMetadataDB        PipelineStepStatus `json:"update_token_metadata_db,omitempty"`
	UpdateJobDB                  PipelineStepStatus `json:"update_job_db,omitempty"`
}

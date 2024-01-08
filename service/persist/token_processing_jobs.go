package persist

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/tracing"
)

type ProcessingCause string

const (
	ProcessingCauseSync          ProcessingCause = "sync"
	ProcessingCauseSyncRetry     ProcessingCause = "sync_retry"
	ProcessingCauseRefresh       ProcessingCause = "refresh"
	ProcessingCausePostPreflight ProcessingCause = "post_preflight"
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

func (t TokenProperties) Value() (driver.Value, error) {
	return json.Marshal(t)
}

func (t *TokenProperties) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), t)
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
	MetadataRetrieval                              PipelineStepStatus `json:"metadata_retrieval,omitempty"`
	TokenInfoRetrieval                             PipelineStepStatus `json:"token_info_retrieval,omitempty"`
	MediaURLsRetrieval                             PipelineStepStatus `json:"media_urls_retrieval,omitempty"`
	AnimationContentHeaderValueRetrieval           PipelineStepStatus `json:"animation_content_header_value_retrieval,omitempty"`
	AnimationReaderRetrieval                       PipelineStepStatus `json:"animation_reader_retrieval,omitempty"`
	AnimationDetermineMediaTypeWithReader          PipelineStepStatus `json:"animation_determine_media_type_with_reader,omitempty"`
	AnimationAnimationGzip                         PipelineStepStatus `json:"animation_animation_gzip,omitempty"`
	AnimationSVGRasterize                          PipelineStepStatus `json:"animation_svg_rasterize,omitempty"`
	AnimationStoreGCP                              PipelineStepStatus `json:"animation_store_gcp,omitempty"`
	AnimationThumbnailGCP                          PipelineStepStatus `json:"animation_thumbnail_gcp,omitempty"`
	AnimationLiveRenderGCP                         PipelineStepStatus `json:"animation_live_render_gcp,omitempty"`
	ImageContentHeaderValueRetrieval               PipelineStepStatus `json:"image_content_header_value_retrieval,omitempty"`
	ImageReaderRetrieval                           PipelineStepStatus `json:"image_reader_retrieval,omitempty"`
	ImageDetermineMediaTypeWithReader              PipelineStepStatus `json:"image_determine_media_type_with_reader,omitempty"`
	ImageAnimationGzip                             PipelineStepStatus `json:"image_animation_gzip,omitempty"`
	ImageSVGRasterize                              PipelineStepStatus `json:"image_svg_rasterize,omitempty"`
	ImageStoreGCP                                  PipelineStepStatus `json:"image_store_gcp,omitempty"`
	ImageThumbnailGCP                              PipelineStepStatus `json:"image_thumbnail_gcp,omitempty"`
	ImageLiveRenderGCP                             PipelineStepStatus `json:"image_live_render_gcp,omitempty"`
	AlternateAnimationContentHeaderValueRetrieval  PipelineStepStatus `json:"alternate_animation_content_header_value_retrieval,omitempty"`
	AlternateAnimationReaderRetrieval              PipelineStepStatus `json:"alternate_animation_reader_retrieval,omitempty"`
	AlternateAnimationDetermineMediaTypeWithReader PipelineStepStatus `json:"alternate_animation_determine_media_type_with_reader,omitempty"`
	AlternateAnimationAnimationGzip                PipelineStepStatus `json:"alternate_animation_animation_gzip,omitempty"`
	AlternateAnimationSVGRasterize                 PipelineStepStatus `json:"alternate_animation_svg_rasterize,omitempty"`
	AlternateAnimationStoreGCP                     PipelineStepStatus `json:"alternate_animation_store_gcp,omitempty"`
	AlternateAnimationThumbnailGCP                 PipelineStepStatus `json:"alternate_animation_thumbnail_gcp,omitempty"`
	AlternateAnimationLiveRenderGCP                PipelineStepStatus `json:"alternate_animation_live_render_gcp,omitempty"`
	AlternateImageContentHeaderValueRetrieval      PipelineStepStatus `json:"alternate_image_content_header_value_retrieval,omitempty"`
	AlternateImageReaderRetrieval                  PipelineStepStatus `json:"alternate_image_reader_retrieval,omitempty"`
	AlternateImageDetermineMediaTypeWithReader     PipelineStepStatus `json:"alternate_image_determine_media_type_with_reader,omitempty"`
	AlternateImageAnimationGzip                    PipelineStepStatus `json:"alternate_image_animation_gzip,omitempty"`
	AlternateImageSVGRasterize                     PipelineStepStatus `json:"alternate_image_svg_rasterize,omitempty"`
	AlternateImageStoreGCP                         PipelineStepStatus `json:"alternate_image_store_gcp,omitempty"`
	AlternateImageThumbnailGCP                     PipelineStepStatus `json:"alternate_image_thumbnail_gcp,omitempty"`
	AlternateImageLiveRenderGCP                    PipelineStepStatus `json:"alternate_image_live_render_gcp,omitempty"`
	ProfileImageContentHeaderValueRetrieval        PipelineStepStatus `json:"pfp_content_header_value_retrieval,omitempty"`
	ProfileImageReaderRetrieval                    PipelineStepStatus `json:"pfp_reader_retrieval,omitempty"`
	ProfileImageDetermineMediaTypeWithReader       PipelineStepStatus `json:"pfp_determine_media_type_with_reader,omitempty"`
	ProfileImageAnimationGzip                      PipelineStepStatus `json:"pfp_animation_gzip,omitempty"`
	ProfileImageSVGRasterize                       PipelineStepStatus `json:"pfp_svg_rasterize,omitempty"`
	ProfileImageStoreGCP                           PipelineStepStatus `json:"pfp_store_gcp,omitempty"`
	ProfileImageThumbnailGCP                       PipelineStepStatus `json:"pfp_thumbnail_gcp,omitempty"`
	ProfileImageLiveRenderGCP                      PipelineStepStatus `json:"pfp_live_render_gcp,omitempty"`
	NothingCachedWithErrors                        PipelineStepStatus `json:"nothing_cached_errors,omitempty"`
	NothingCachedWithoutErrors                     PipelineStepStatus `json:"nothing_cached_no_errors,omitempty"`
	CreateMedia                                    PipelineStepStatus `json:"create_media,omitempty"`
	CreateMediaFromCachedObjects                   PipelineStepStatus `json:"create_media_from_cached_objects,omitempty"`
	CreateRawMedia                                 PipelineStepStatus `json:"create_raw_media,omitempty"`
	MediaResultComparison                          PipelineStepStatus `json:"media_result_comparison,omitempty"`
}

func (p PipelineMetadata) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *PipelineMetadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), p)
}

func TrackStepStatus(ctx context.Context, status *PipelineStepStatus, name string) (func(), context.Context) {
	// Keep track of the parent step name
	if log := logger.For(ctx); log != nil {
		if parent, ok := log.Data["pipelineStep"].(string); ok && parent != "" {
			name = fmt.Sprintf("%s.%s", parent, name)
		}
	}

	span, ctx := tracing.StartSpan(ctx, "pipeline.step", name)

	ctx = logger.NewContextWithFields(ctx, logrus.Fields{
		"pipelineStep": name,
	})

	startTime := time.Now()

	if status == nil {
		started := PipelineStepStatusStarted
		status = &started
	}
	*status = PipelineStepStatusStarted

	go func() {
		for {
			<-time.After(5 * time.Second)
			if status == nil || *status == PipelineStepStatusSuccess || *status == PipelineStepStatusError {
				return
			}
			logger.For(ctx).Infof("still on [%s] (taken: %s)", name, time.Since(startTime))
		}
	}()

	return func() {
		defer tracing.FinishSpan(span)
		if *status == PipelineStepStatusError {
			logger.For(ctx).Errorf("failed [%s] (took: %s)", name, time.Since(startTime))
			return
		}
		*status = PipelineStepStatusSuccess
		logger.For(ctx).Infof("succeeded [%s] (took: %s)", name, time.Since(startTime))
	}, ctx
}

func FailStep(status *PipelineStepStatus) {
	if status == nil {
		errored := PipelineStepStatusError
		status = &errored
	}
	*status = PipelineStepStatusError
}

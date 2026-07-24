package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var errStudioMediaIncomplete = errors.New("studio media publication incomplete")

const (
	studioMaxMediaTracks    = 200
	studioMaxMediaSubtitles = 32
	studioMaxMediaChapters  = 500
)

// StudioMediaMetadata is the neutral package contract shared by Cabinet and
// Moments. Every asset ID points to private Studio storage until publication.
type StudioMediaMetadata struct {
	Collection           string                `json:"collection,omitempty"`
	Date                 string                `json:"date,omitempty"`
	Contributor          string                `json:"contributor,omitempty"`
	License              string                `json:"license,omitempty"`
	PrimaryAssetID       string                `json:"primaryAssetId,omitempty"`
	PrimaryName          string                `json:"primaryName,omitempty"`
	CoverAssetID         string                `json:"coverAssetId,omitempty"`
	CoverName            string                `json:"coverName,omitempty"`
	ChannelAvatarAssetID string                `json:"channelAvatarAssetId,omitempty"`
	ChannelAvatarName    string                `json:"channelAvatarName,omitempty"`
	Duration             int                   `json:"duration,omitempty"`
	Tracks               []StudioMediaTrack    `json:"tracks,omitempty"`
	Subtitles            []StudioMediaSubtitle `json:"subtitles,omitempty"`
	Chapters             []StudioMediaChapter  `json:"chapters,omitempty"`
}

type StudioMediaTrack struct {
	Title           string `json:"title"`
	AssetID         string `json:"assetId"`
	WaveformAssetID string `json:"waveformAssetId,omitempty"`
}

type StudioMediaSubtitle struct {
	Lang    string `json:"lang"`
	AssetID string `json:"assetId"`
}

type StudioMediaChapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

func validateStudioMediaMetadata(template string, raw json.RawMessage) (StudioMediaMetadata, []string, error) {
	if !strings.HasPrefix(template, "cabinet.") && template != "moments.video" {
		return StudioMediaMetadata{}, nil, nil
	}
	var metadata StudioMediaMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return metadata, nil, fmt.Errorf("metadata: invalid media profile")
	}
	metadata.Collection = strings.TrimSpace(metadata.Collection)
	if metadata.Collection == "" {
		metadata.Collection = "General"
	}
	if utf8.RuneCountInString(metadata.Collection) > 120 || sanitizeSegment(metadata.Collection) == "" {
		return metadata, nil, fmt.Errorf("metadata.collection: invalid")
	}
	metadata.Collection = sanitizeSegment(metadata.Collection)
	for name, value := range map[string]string{
		"date": metadata.Date, "contributor": metadata.Contributor, "license": metadata.License,
		"primaryName": metadata.PrimaryName, "coverName": metadata.CoverName,
		"channelAvatarName": metadata.ChannelAvatarName,
	} {
		if utf8.RuneCountInString(strings.TrimSpace(value)) > 500 {
			return metadata, nil, fmt.Errorf("metadata.%s: too long", name)
		}
	}
	metadata.Date = strings.TrimSpace(metadata.Date)
	metadata.Contributor = strings.TrimSpace(metadata.Contributor)
	metadata.License = strings.TrimSpace(metadata.License)
	metadata.PrimaryName = strings.TrimSpace(metadata.PrimaryName)
	metadata.CoverName = strings.TrimSpace(metadata.CoverName)
	metadata.ChannelAvatarName = strings.TrimSpace(metadata.ChannelAvatarName)
	// Cabinet Audio used to store one file separately as primaryAssetId while
	// the player only enumerated tracks. Normalize that legacy shape so the
	// former primary file becomes track 1 and every audio follows one ordering.
	if template == "cabinet.audio" && strings.TrimSpace(metadata.PrimaryAssetID) != "" {
		primaryID := strings.TrimSpace(metadata.PrimaryAssetID)
		index := -1
		for i := range metadata.Tracks {
			if strings.TrimSpace(metadata.Tracks[i].AssetID) == primaryID {
				index = i
				break
			}
		}
		var primaryTrack StudioMediaTrack
		if index >= 0 {
			primaryTrack = metadata.Tracks[index]
			metadata.Tracks = append(metadata.Tracks[:index], metadata.Tracks[index+1:]...)
		} else {
			title := strings.TrimSpace(strings.TrimSuffix(
				metadata.PrimaryName, filepath.Ext(metadata.PrimaryName)))
			if title == "" {
				title = "Audio"
			}
			primaryTrack = StudioMediaTrack{Title: title, AssetID: primaryID}
		}
		metadata.Tracks = append([]StudioMediaTrack{primaryTrack}, metadata.Tracks...)
		metadata.PrimaryAssetID = ""
		metadata.PrimaryName = ""
	}
	if metadata.Duration < 0 || metadata.Duration > 7*24*60*60 {
		return metadata, nil, fmt.Errorf("metadata.duration: invalid")
	}
	if len(metadata.Tracks) > studioMaxMediaTracks ||
		len(metadata.Subtitles) > studioMaxMediaSubtitles ||
		len(metadata.Chapters) > studioMaxMediaChapters {
		return metadata, nil, fmt.Errorf("metadata: too many entries")
	}

	assets := map[string]bool{}
	addAsset := func(field, id string, required bool) error {
		id = strings.TrimSpace(id)
		if id == "" && !required {
			return nil
		}
		if !studioIDRE.MatchString(id) {
			return fmt.Errorf("metadata.%s: invalid asset", field)
		}
		assets[id] = true
		return nil
	}
	if err := addAsset("primaryAssetId", metadata.PrimaryAssetID, false); err != nil {
		return metadata, nil, err
	}
	if err := addAsset("coverAssetId", metadata.CoverAssetID, false); err != nil {
		return metadata, nil, err
	}
	if err := addAsset("channelAvatarAssetId", metadata.ChannelAvatarAssetID, false); err != nil {
		return metadata, nil, err
	}
	for i := range metadata.Tracks {
		track := &metadata.Tracks[i]
		track.Title = strings.TrimSpace(track.Title)
		if track.Title == "" || utf8.RuneCountInString(track.Title) > 240 {
			return metadata, nil, fmt.Errorf("metadata.tracks[%d].title: invalid", i)
		}
		if err := addAsset(fmt.Sprintf("tracks[%d].assetId", i), track.AssetID, true); err != nil {
			return metadata, nil, err
		}
		if err := addAsset(fmt.Sprintf("tracks[%d].waveformAssetId", i), track.WaveformAssetID, false); err != nil {
			return metadata, nil, err
		}
	}
	for i := range metadata.Subtitles {
		subtitle := &metadata.Subtitles[i]
		subtitle.Lang = strings.TrimSpace(subtitle.Lang)
		if subtitle.Lang == "" || utf8.RuneCountInString(subtitle.Lang) > 32 {
			return metadata, nil, fmt.Errorf("metadata.subtitles[%d].lang: invalid", i)
		}
		if err := addAsset(fmt.Sprintf("subtitles[%d].assetId", i), subtitle.AssetID, true); err != nil {
			return metadata, nil, err
		}
	}
	previous := -1.0
	for i := range metadata.Chapters {
		chapter := &metadata.Chapters[i]
		chapter.Title = strings.TrimSpace(chapter.Title)
		if chapter.Start < 0 || chapter.Start < previous ||
			chapter.Title == "" || utf8.RuneCountInString(chapter.Title) > 240 {
			return metadata, nil, fmt.Errorf("metadata.chapters[%d]: invalid", i)
		}
		previous = chapter.Start
	}
	if template != "cabinet.audio" && len(metadata.Tracks) != 0 {
		return metadata, nil, fmt.Errorf("metadata.tracks: unsupported for template")
	}
	videoProfile := template == "moments.video" || template == "cabinet.video"
	if !videoProfile && len(metadata.Subtitles) != 0 {
		return metadata, nil, fmt.Errorf("metadata.subtitles: unsupported for template")
	}
	if !videoProfile && len(metadata.Chapters) != 0 {
		return metadata, nil, fmt.Errorf("metadata.chapters: unsupported for template")
	}
	out := make([]string, 0, len(assets))
	for id := range assets {
		out = append(out, id)
	}
	return metadata, out, nil
}

func studioMediaReadyForPublication(template string, metadata StudioMediaMetadata) error {
	if metadata.PrimaryAssetID == "" && !(template == "cabinet.audio" && len(metadata.Tracks) > 0) {
		return fmt.Errorf("%w: primary file required", errStudioMediaIncomplete)
	}
	if template == "moments.video" && metadata.CoverAssetID == "" {
		return fmt.Errorf("%w: thumbnail required", errStudioMediaIncomplete)
	}
	return nil
}

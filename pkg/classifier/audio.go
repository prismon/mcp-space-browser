package classifier

import (
	"fmt"
	"os"

	"github.com/dhowden/tag"
)

// AudioClassifier extracts metadata from audio files (MP3, FLAC, M4A, OGG, etc.)
type AudioClassifier struct{}

// NewAudioClassifier creates a new audio classifier
func NewAudioClassifier() *AudioClassifier {
	return &AudioClassifier{}
}

// Name returns the classifier name
func (a *AudioClassifier) Name() string {
	return "Audio Metadata Extractor"
}

// CanHandle checks if this is an audio file
func (a *AudioClassifier) CanHandle(filePath string) bool {
	return DetectFileType(filePath) == FileTypeAudio
}

// IsAvailable returns true (always available, uses go-tag library)
func (a *AudioClassifier) IsAvailable() bool {
	return true
}

// Extract extracts audio metadata including ID3 tags
func (a *AudioClassifier) Extract(req *MetadataRequest) *MetadataResult {
	result := &MetadataResult{
		MetadataType: "audio-metadata",
		Data:         make(map[string]interface{}),
	}

	// Open the file
	file, err := os.Open(req.SourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to open file: %w", err)
		return result
	}
	defer file.Close()

	// Get file stats
	stat, err := file.Stat()
	if err != nil {
		result.Error = fmt.Errorf("failed to stat file: %w", err)
		return result
	}

	result.Data["file_size"] = stat.Size()

	// Read metadata tags
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		result.Error = fmt.Errorf("failed to read audio metadata: %w", err)
		return result
	}

	// Extract basic metadata
	result.Data["format"] = string(metadata.Format())
	result.Data["file_type"] = string(metadata.FileType())

	// Extract ID3 tags
	if title := metadata.Title(); title != "" {
		result.Data["title"] = title
	}
	if artist := metadata.Artist(); artist != "" {
		result.Data["artist"] = artist
	}
	if album := metadata.Album(); album != "" {
		result.Data["album"] = album
	}
	if albumArtist := metadata.AlbumArtist(); albumArtist != "" {
		result.Data["album_artist"] = albumArtist
	}
	if composer := metadata.Composer(); composer != "" {
		result.Data["composer"] = composer
	}
	if genre := metadata.Genre(); genre != "" {
		result.Data["genre"] = genre
	}
	if year := metadata.Year(); year != 0 {
		result.Data["year"] = year
	}

	// Track information
	track, totalTracks := metadata.Track()
	if track != 0 {
		result.Data["track_number"] = track
	}
	if totalTracks != 0 {
		result.Data["total_tracks"] = totalTracks
	}

	disc, totalDiscs := metadata.Disc()
	if disc != 0 {
		result.Data["disc_number"] = disc
	}
	if totalDiscs != 0 {
		result.Data["total_discs"] = totalDiscs
	}

	// Lyrics (if available)
	if lyrics := metadata.Lyrics(); lyrics != "" {
		// Store only first 500 characters of lyrics to avoid bloat
		if len(lyrics) > 500 {
			result.Data["lyrics_preview"] = lyrics[:500] + "..."
			result.Data["has_full_lyrics"] = true
		} else {
			result.Data["lyrics"] = lyrics
		}
	}

	// Comment
	if comment := metadata.Comment(); comment != "" {
		result.Data["comment"] = comment
	}

	// Album artwork
	picture := metadata.Picture()
	if picture != nil {
		result.Data["has_artwork"] = true
		result.Data["artwork_mime_type"] = picture.MIMEType
		result.Data["artwork_size"] = len(picture.Data)
		// Don't store the actual image data in metadata - it's too large
		// The thumbnail can be extracted separately as an artifact
	} else {
		result.Data["has_artwork"] = false
	}

	return result
}

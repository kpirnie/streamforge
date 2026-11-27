// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

// Stream describes the media stream attributes used for validation and conversion.
type Stream struct {
	Codec       string        `json:"codec_name"`
	Type        string        `json:"codec_type"`
	Transfer    string        `json:"color_transfer"`
	PixelFormat string        `json:"pix_fmt"`
	Primaries   string        `json:"color_primaries"`
	ColorSpace  string        `json:"color_space"`
	ColorRange  string        `json:"color_range"`
	SideData    []hdrSideData `json:"side_data_list"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
	Tags struct {
		Language string `json:"language"`
	} `json:"tags"`
}

// Probe validates a media file with the host-mounted FFprobe executable.
func Probe(ctx context.Context, binary string, file *os.File) ([]Stream, error) {
	args := []string{"-v", "error", "-protocol_whitelist", mediaProtocols, "-show_entries", "stream=codec_name,codec_type,color_transfer,pix_fmt,color_primaries,color_space,color_range:stream_side_data:stream_disposition=attached_pic:stream_tags=language", "-of", "json"}
	args = append(args, mediaFileArgs(0)...)
	command := mediaCommand(ctx, binary, args...)
	if err := configureMediaFiles(command, file, nil, nil); err != nil {
		return nil, err
	}
	output, err := command.Output()
	if err != nil {
		return nil, mediaCommandError(ctx, err, "FFprobe could not read media")
	}
	var result struct {
		Streams []Stream `json:"streams"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, errors.New("invalid FFprobe stream report")
	}
	for _, stream := range result.Streams {
		if stream.Type == "video" && stream.Codec != "" {
			return result.Streams, nil
		}
	}
	return nil, errors.New("media does not contain a readable video stream")
}

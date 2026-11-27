// Stream Forge - Terminal media administration
// Copyright (c) 2026 Kevin Pirnie <iam@kevinpirnie.com>
// Licensed under the MIT License. See LICENSE file in the project root for full license text.

package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
)

type hdrSideData struct {
	Type         string `json:"side_data_type"`
	RedX         string `json:"red_x"`
	RedY         string `json:"red_y"`
	GreenX       string `json:"green_x"`
	GreenY       string `json:"green_y"`
	BlueX        string `json:"blue_x"`
	BlueY        string `json:"blue_y"`
	WhiteX       string `json:"white_point_x"`
	WhiteY       string `json:"white_point_y"`
	MaxLuminance string `json:"max_luminance"`
	MinLuminance string `json:"min_luminance"`
	MaxContent   int    `json:"max_content"`
	MaxAverage   int    `json:"max_average"`
}

func (c Conversion) hdrOptions(ctx context.Context, file *os.File, stream Stream) ([]string, error) {
	command := mediaCommand(ctx, c.FFprobe, "-v", "error", "-protocol_whitelist", "file,pipe", "-select_streams", "v:0", "-read_intervals", "%+#24", "-show_frames", "-show_entries", "frame=side_data_list:frame_side_data", "-of", "json", "/proc/self/fd/3")
	command.ExtraFiles = []*os.File{file}
	data, err := command.Output()
	if err != nil {
		return nil, mediaCommandError(ctx, err, "could not inspect HDR side data")
	}
	var result struct {
		Frames []struct {
			SideData []hdrSideData `json:"side_data_list"`
		}
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("invalid HDR side data report")
	}
	if len(result.Frames) == 0 {
		return nil, errors.New("could not verify source HDR frames")
	}
	sideData := append([]hdrSideData(nil), stream.SideData...)
	for _, frame := range result.Frames {
		sideData = append(sideData, frame.SideData...)
	}

	mastering, light := "", ""
	for _, side := range sideData {
		kind := strings.ToLower(side.Type)
		switch {
		case strings.Contains(kind, "dolby"), strings.Contains(kind, "dovi"), strings.Contains(kind, "2094"), strings.Contains(kind, "dynamic hdr"):
			return nil, errors.New("dynamic HDR requires stream copy or explicit SDR conversion")
		case strings.Contains(kind, "mastering display"):
			mastering, err = masteringDisplay(side)
			if err != nil {
				return nil, err
			}
		case strings.Contains(kind, "content light") && side.MaxContent >= 0 && side.MaxAverage >= 0:
			light = fmt.Sprintf("%d,%d", side.MaxContent, side.MaxAverage)
		}
	}
	parameters := []string{"repeat-headers=1", fmt.Sprintf("pools=%d", max(1, c.Threads)), "frame-threads=1"}
	if stream.Transfer == "smpte2084" {
		parameters = append(parameters, "hdr10=1", "hdr10-opt=1")
	}
	if mastering != "" {
		parameters = append(parameters, "master-display="+mastering)
	}
	if light != "" {
		parameters = append(parameters, "max-cll="+light)
	}
	args := []string{"-pix_fmt", stream.PixelFormat, "-color_trc", stream.Transfer, "-x265-params", strings.Join(parameters, ":")}
	if stream.Primaries != "" && stream.Primaries != "unknown" {
		args = append(args, "-color_primaries", stream.Primaries)
	}
	if stream.ColorSpace != "" && stream.ColorSpace != "unknown" {
		args = append(args, "-colorspace", stream.ColorSpace)
	}
	if stream.ColorRange != "" && stream.ColorRange != "unknown" {
		args = append(args, "-color_range", stream.ColorRange)
	}
	return args, nil
}

func masteringDisplay(data hdrSideData) (string, error) {
	values := []string{data.GreenX, data.GreenY, data.BlueX, data.BlueY, data.RedX, data.RedY, data.WhiteX, data.WhiteY, data.MaxLuminance, data.MinLuminance}
	var scaled [10]int64
	for i, value := range values {
		ratio, ok := new(big.Rat).SetString(value)
		if !ok || ratio.Sign() < 0 {
			return "", errors.New("invalid HDR mastering display metadata")
		}
		units := int64(50000)
		if i >= 8 {
			units = 10000
		}
		ratio.Mul(ratio, big.NewRat(units, 1))
		rounded := new(big.Int).Quo(ratio.Num(), ratio.Denom())
		if !rounded.IsInt64() {
			return "", errors.New("invalid HDR mastering display range")
		}
		scaled[i] = rounded.Int64()
	}
	return fmt.Sprintf("G(%d,%d)B(%d,%d)R(%d,%d)WP(%d,%d)L(%d,%d)", scaled[0], scaled[1], scaled[2], scaled[3], scaled[4], scaled[5], scaled[6], scaled[7], scaled[8], scaled[9]), nil
}

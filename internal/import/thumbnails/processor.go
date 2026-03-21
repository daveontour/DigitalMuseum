package thumbnails

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ExifData represents the ImageMagick format string output.
type ExifData struct {
	Width            string   `json:"width"`
	Height           string   `json:"height"`
	DateTaken        string   `json:"date_taken"`
	Latitude         string   `json:"latitude"`
	Longitude        string   `json:"longitude"`
	LatitudeRef      string   `json:"latitude_ref"`
	LongitudeRef     string   `json:"longitude_ref"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Tags             string   `json:"tags"`
	LatitudeDecimal  *float64 `json:"-"` // Parsed decimal degrees (S = negative)
	LongitudeDecimal *float64 `json:"-"` // Parsed decimal degrees (W = negative)
}

// Processor creates thumbnails and extracts EXIF via ImageMagick.
type Processor struct{}

func (p *Processor) findImageMagickCommand() string {
	return "magick"
}

// parseGPSCoordinate parses GPS coordinate from ImageMagick format (degrees/minutes/seconds as fractions)
// to decimal degrees. Format: "degrees/num,minutes/num,seconds/num" e.g. "25/1,6/1,4036/100" = 25° 6' 40.36"
func parseGPSCoordinate(gpsString string) *float64 {
	gpsString = strings.TrimSpace(gpsString)
	if gpsString == "" {
		return nil
	}
	parts := strings.Split(gpsString, ",")
	if len(parts) != 3 {
		return nil
	}
	// Parse degrees: "25/1" -> 25.0
	degParts := strings.Split(strings.TrimSpace(parts[0]), "/")
	var degrees float64
	if len(degParts) == 2 {
		num, err1 := strconv.ParseFloat(strings.TrimSpace(degParts[0]), 64)
		den, err2 := strconv.ParseFloat(strings.TrimSpace(degParts[1]), 64)
		if err1 != nil || err2 != nil || den == 0 {
			return nil
		}
		degrees = num / den
	} else {
		val, err := strconv.ParseFloat(strings.TrimSpace(degParts[0]), 64)
		if err != nil {
			return nil
		}
		degrees = val
	}
	// Parse minutes: "6/1" -> 6.0
	minParts := strings.Split(strings.TrimSpace(parts[1]), "/")
	var minutes float64
	if len(minParts) == 2 {
		num, err1 := strconv.ParseFloat(strings.TrimSpace(minParts[0]), 64)
		den, err2 := strconv.ParseFloat(strings.TrimSpace(minParts[1]), 64)
		if err1 != nil || err2 != nil || den == 0 {
			return nil
		}
		minutes = num / den
	} else {
		val, err := strconv.ParseFloat(strings.TrimSpace(minParts[0]), 64)
		if err != nil {
			return nil
		}
		minutes = val
	}
	// Parse seconds: "4036/100" -> 40.36
	secParts := strings.Split(strings.TrimSpace(parts[2]), "/")
	var seconds float64
	if len(secParts) == 2 {
		num, err1 := strconv.ParseFloat(strings.TrimSpace(secParts[0]), 64)
		den, err2 := strconv.ParseFloat(strings.TrimSpace(secParts[1]), 64)
		if err1 != nil || err2 != nil || den == 0 {
			return nil
		}
		seconds = num / den
	} else {
		val, err := strconv.ParseFloat(strings.TrimSpace(secParts[0]), 64)
		if err != nil {
			return nil
		}
		seconds = val
	}
	decimal := degrees + (minutes / 60.0) + (seconds / 3600.0)
	return &decimal
}

func (p *Processor) parseExifData(data ExifData) ExifData {
	// Parse latitude: convert DMS to decimal and apply LatitudeRef (S = negative)
	if s := strings.TrimSpace(data.Latitude); s != "" {
		if dec := parseGPSCoordinate(s); dec != nil {
			ref := strings.TrimSpace(strings.ToUpper(data.LatitudeRef))
			if ref == "S" {
				v := -*dec
				data.LatitudeDecimal = &v
			} else {
				data.LatitudeDecimal = dec
			}
		}
	}
	// Parse longitude: convert DMS to decimal and apply LongitudeRef (W = negative)
	if s := strings.TrimSpace(data.Longitude); s != "" {
		if dec := parseGPSCoordinate(s); dec != nil {
			ref := strings.TrimSpace(strings.ToUpper(data.LongitudeRef))
			if ref == "W" {
				v := -*dec
				data.LongitudeDecimal = &v
			} else {
				data.LongitudeDecimal = dec
			}
		}
	}
	return data
}

// CreateThumbAndGetExif generates a thumbnail and/or extracts EXIF from image bytes via ImageMagick.
func (p *Processor) CreateThumbAndGetExif(
	imageData []byte,
	processThumbnail bool,
	processExif bool,
	width int,
) ([]byte, *ExifData, error) {
	magickCmd := p.findImageMagickCommand()
	formatString := `{"width": "%w", "height": "%h", "date_taken": "%[EXIF:DateTimeOriginal]", "latitude": "%[EXIF:GPSLatitude]", "longitude": "%[EXIF:GPSLongitude]", "latitude_ref": "%[EXIF:GPSLatitudeRef]", "longitude_ref": "%[EXIF:GPSLongitudeRef]", "title": "%[EXIF:DocumentName]", "description": "%[EXIF:ImageDescription]", "tags": "%[EXIF:Keywords]"}`

	if processThumbnail && processExif {
		args := []string{
			"-",
			"-quiet",
			"-format", formatString,
			"-write", "info:fd:2",
			"-filter", "Lanczos",
			"-colorspace", "sRGB",
			"-resize", fmt.Sprintf("%dx%d>", width, width),
			"-unsharp", "0x0.75+0.75+0.008",
			"-quality", "95",
			"-strip",
			"jpg:-",
		}

		cmd := exec.Command(magickCmd, args...)
		hideConsole(cmd)
		cmd.Stdin = bytes.NewReader(imageData)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return nil, nil, fmt.Errorf("ImageMagick Error: %v, Stderr: %s", err, stderr.String())
		}

		var exif ExifData
		if err := json.Unmarshal(stderr.Bytes(), &exif); err != nil {
			return stdout.Bytes(), nil, nil // Return image even if EXIF fails
		}

		parsedExif := p.parseExifData(exif)
		return stdout.Bytes(), &parsedExif, nil

	} else if processThumbnail {
		args := []string{
			"-",
			"-filter", "Lanczos",
			"-colorspace", "sRGB",
			"-resize", fmt.Sprintf("%dx%d>", width, width),
			"-unsharp", "0x0.75+0.75+0.008",
			"-quality", "95",
			"-strip",
			"jpg:-",
		}

		cmd := exec.Command(magickCmd, args...)
		hideConsole(cmd)
		cmd.Stdin = bytes.NewReader(imageData)
		output, err := cmd.Output()
		if err != nil {
			return nil, nil, err
		}
		return output, nil, nil

	} else if processExif {
		args := []string{
			"identify",
			"-quiet",
			"-format", formatString,
			"-",
		}

		cmd := exec.Command(magickCmd, args...)
		hideConsole(cmd)
		cmd.Stdin = bytes.NewReader(imageData)
		output, err := cmd.Output()
		if err != nil {
			return nil, nil, err
		}

		var exif ExifData
		if err := json.Unmarshal(output, &exif); err != nil {
			return nil, nil, err
		}

		parsedExif := p.parseExifData(exif)
		return nil, &parsedExif, nil
	}

	return nil, nil, nil
}

package area

import (
	"errors"
	"strings"
)

const (
	ModeBBox   Mode = "bbox"
	ModeRegion Mode = "region"

	ZoomMin = 0
	ZoomMax = 20

	maxMercatorLatitude = 85.05112878
)

type Mode string

type Spec struct {
	Mode     Mode   `json:"mode"`
	BBox     *BBox  `json:"bbox,omitempty"`
	GeoJSON  string `json:"geojson,omitempty"`
	RegionID string `json:"regionId,omitempty"`
}

type BBox struct {
	MinLon float64 `json:"minLon"`
	MinLat float64 `json:"minLat"`
	MaxLon float64 `json:"maxLon"`
	MaxLat float64 `json:"maxLat"`
}

type ZoomRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func Normalize(spec Spec, fallbackMode Mode) (Spec, error) {
	if spec.Mode == "" {
		spec.Mode = fallbackMode
	}
	switch spec.Mode {
	case ModeBBox:
		if spec.BBox == nil {
			return Spec{}, errors.New("bbox area is required")
		}
		if err := spec.BBox.Validate(); err != nil {
			return Spec{}, err
		}
		spec.GeoJSON = ""
		spec.RegionID = ""
	case ModeRegion:
		spec.GeoJSON = strings.TrimSpace(spec.GeoJSON)
		spec.RegionID = strings.TrimSpace(spec.RegionID)
		if spec.GeoJSON == "" && spec.RegionID == "" {
			return Spec{}, errors.New("region area requires geojson or regionId")
		}
		spec.BBox = nil
	default:
		return Spec{}, errors.New("mode must be bbox or region")
	}
	return spec, nil
}

func (box BBox) Validate() error {
	if box.MinLon < -180 || box.MinLon > 180 || box.MaxLon < -180 || box.MaxLon > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	if box.MinLat < -maxMercatorLatitude || box.MinLat > maxMercatorLatitude || box.MaxLat < -maxMercatorLatitude || box.MaxLat > maxMercatorLatitude {
		return errors.New("latitude must be between -85.05112878 and 85.05112878")
	}
	if box.MinLon >= box.MaxLon {
		return errors.New("minLon must be less than maxLon")
	}
	if box.MinLat >= box.MaxLat {
		return errors.New("minLat must be less than maxLat")
	}
	return nil
}

func (zoom ZoomRange) Validate() error {
	if zoom.Min < ZoomMin || zoom.Max > ZoomMax {
		return errors.New("zoom level out of supported range")
	}
	if zoom.Min > zoom.Max {
		return errors.New("min zoom cannot be greater than max zoom")
	}
	return nil
}

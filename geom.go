package search

import "math"

func validateBox(b Box) error {
	if err := finitePair(b.West, b.East); err != nil {
		return ErrBox
	}
	if err := finitePair(b.South, b.North); err != nil {
		return ErrBox
	}
	if b.West > b.East || b.South > b.North {
		return ErrBox
	}
	if b.West < -180 || b.East > 180 || b.South < -90 || b.North > 90 {
		return ErrBox
	}
	return nil
}

func validatePoint(p Point) error {
	if err := finitePair(p.Lon, p.Lat); err != nil {
		return ErrBox
	}
	if p.Lon < -180 || p.Lon > 180 || p.Lat < -90 || p.Lat > 90 {
		return ErrBox
	}
	return nil
}

func validateViewport(b Box) error {
	if err := finitePair(b.West, b.East); err != nil {
		return ErrViewport
	}
	if err := finitePair(b.South, b.North); err != nil {
		return ErrViewport
	}
	if b.South > b.North || b.South < -90 || b.North > 90 {
		return ErrViewport
	}
	if b.West < -180 || b.West > 180 || b.East < -180 || b.East > 180 {
		return ErrViewport
	}
	return nil
}

func finitePair(a, b float64) error {
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return ErrInvalid
	}
	return nil
}

func unionBox(a, b Box) Box {
	return Box{
		West:  math.Min(a.West, b.West),
		South: math.Min(a.South, b.South),
		East:  math.Max(a.East, b.East),
		North: math.Max(a.North, b.North),
	}
}

func validateQuery(q Query) error {
	if q.Geometry != GeometryShown && q.Geometry != GeometryExact {
		return ErrGeometry
	}
	if q.Viewport != nil {
		if err := validateViewport(*q.Viewport); err != nil {
			return err
		}
	}
	return validateWindow(q.BeginsAt, q.EndsBefore)
}

func validateWindow(beginsAt, endsBefore int64) error {
	if (beginsAt == 0) != (endsBefore == 0) {
		return ErrWindow
	}
	if beginsAt < 0 || endsBefore < 0 {
		return ErrWindow
	}
	if beginsAt != 0 && endsBefore <= beginsAt {
		return ErrWindow
	}
	return nil
}

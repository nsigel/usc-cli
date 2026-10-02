package libcal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nsigel/usc-cli/config"
)

const reservationsVersion = 1

type savedReservations struct {
	Version      int           `json:"version"`
	Reservations []Reservation `json:"reservations"`
}

// Reservations returns the private local history without opening an
// authenticated HTTP session.
func Reservations(includePast bool) ([]Reservation, error) {
	path, err := config.LibCalReservationsPath()
	if err != nil {
		return nil, err
	}
	return reservationsAt(path, includePast)
}

// Reservations reads this client's local booking history. OpenWithOptions
// selects its location alongside the session file. This is not a complete
// list of bookings made through LibCal or other clients.
func (c *Client) Reservations(includePast bool) ([]Reservation, error) {
	if c == nil || c.reservationsPath == "" {
		return nil, errors.New("local reservation history requires libcal.Open or libcal.OpenWithOptions")
	}
	return reservationsAt(c.reservationsPath, includePast)
}

func reservationsAt(path string, includePast bool) ([]Reservation, error) {
	saved, err := readReservations(path)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	result := make([]Reservation, 0, len(saved.Reservations))
	for _, reservation := range saved.Reservations {
		if includePast || reservation.End.After(now) {
			result = append(result, reservation)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Start.Before(result[j].Start) })
	return result, nil
}

func (c *Client) recordReservation(reservation Reservation) error {
	if c.reservationsPath == "" {
		return nil
	}
	saved, err := readReservations(c.reservationsPath)
	if err != nil {
		return err
	}
	for _, existing := range saved.Reservations {
		if existing.Space.ID == reservation.Space.ID && existing.Start.Equal(reservation.Start) {
			return nil
		}
	}
	saved.Reservations = append(saved.Reservations, reservation)
	sort.Slice(saved.Reservations, func(i, j int) bool {
		return saved.Reservations[i].Start.Before(saved.Reservations[j].Start)
	})
	return writeReservations(c.reservationsPath, saved)
}

func readReservations(path string) (savedReservations, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return savedReservations{Version: reservationsVersion, Reservations: []Reservation{}}, nil
	}
	if err != nil {
		return savedReservations{}, err
	}
	var saved savedReservations
	if err := json.Unmarshal(data, &saved); err != nil {
		return savedReservations{}, fmt.Errorf("read local LibCal reservations: %w", err)
	}
	if saved.Version != reservationsVersion {
		return savedReservations{}, fmt.Errorf("unsupported LibCal reservations version %d", saved.Version)
	}
	if saved.Reservations == nil {
		saved.Reservations = []Reservation{}
	}
	return saved, nil
}

func writeReservations(path string, saved savedReservations) error {
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".libcal-reservations-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

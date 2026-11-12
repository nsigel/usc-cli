package classes

import (
	"fmt"
	"regexp"
	"strings"
)

// CourseCode identifies a course within a term.
type CourseCode struct {
	// Prefix is the program the course belongs to: "CSCI".
	Prefix string `json:"prefix"`
	// Number is the course number: "794".
	Number string `json:"number"`
	// Sequence is the capital letter that tells apart the courses of a
	// multi-part series sharing a number: CSCI-794A, CSCI-794B, ... are
	// different courses. Empty for most courses.
	Sequence string `json:"sequence,omitempty"`
}

// String returns the canonical form, "CSCI-102" or "CSCI-794A", which both
// the Schedule of Classes and WebReg accept.
func (code CourseCode) String() string {
	return code.Prefix + "-" + code.Number + code.Sequence
}

// courseCodePattern separates a sequence letter from the catalog suffix that
// may follow it. USC's suffix letters are a capital L (lab) and lowercase
// letters (g, m, p, w, x); sequence letters are the other capitals.
var courseCodePattern = regexp.MustCompile(`^([A-Za-z]+)-?([0-9]+)([A-KM-Z]?)(L?[a-z]*)$`)

// ParseCourseCode reads a course code as USC writes it, with or without the
// hyphen, spaces, and catalog suffix: "CSCI-102", "CSCI 102L", "BUAD-200 x",
// "CSCI-794A", "CSCI-491BL". The catalog suffix describes the course rather
// than identifying it and is dropped; Course.Suffix carries it. The letters
// after the number are case-sensitive, since only case tells a sequence
// letter from a suffix.
func ParseCourseCode(text string) (CourseCode, error) {
	parts := courseCodePattern.FindStringSubmatch(strings.Join(strings.Fields(text), ""))
	if parts == nil {
		return CourseCode{}, fmt.Errorf("course code must look like CSCI-102 or CSCI-794A, got %q", text)
	}
	return CourseCode{
		Prefix:   strings.ToUpper(parts[1]),
		Number:   parts[2],
		Sequence: parts[3],
	}, nil
}

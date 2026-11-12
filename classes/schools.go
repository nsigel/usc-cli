package classes

import (
	"context"
	"net/url"
)

// School is a USC school and the programs it offers in a term.
type School struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
	// Programs are in the order USC lists them. A program can appear under
	// more than one school.
	Programs []Program `json:"programs"`
}

// Program is a subject such as CSCI (Computer Science). Its prefix is the
// first part of its courses' codes.
type Program struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

type schoolResponse struct {
	Prefix   string            `json:"prefix"`
	Name     string            `json:"name"`
	Programs []programResponse `json:"programs"`
}

type programResponse struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

// Schools lists a term's schools with their programs, in the order USC lists
// them.
func (c *Client) Schools(ctx context.Context, termCode string) ([]School, error) {
	if err := validateTermCode(termCode); err != nil {
		return nil, err
	}
	var wire []schoolResponse
	if err := c.get(ctx, "Schools/TermCode", url.Values{"termCode": {termCode}}, &wire); err != nil {
		return nil, err
	}
	schools := make([]School, len(wire))
	for schoolIndex, wireSchool := range wire {
		school := School{
			Prefix:   wireSchool.Prefix,
			Name:     wireSchool.Name,
			Programs: make([]Program, len(wireSchool.Programs)),
		}
		for programIndex, wireProgram := range wireSchool.Programs {
			school.Programs[programIndex] = Program(wireProgram)
		}
		schools[schoolIndex] = school
	}
	return schools, nil
}

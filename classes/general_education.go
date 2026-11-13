package classes

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// StudentGroup is the students a general education requirement applies to:
// students follow the program of the period they entered USC in.
type StudentGroup string

const (
	StudentsEnteringBeforeFall2015  StudentGroup = "BeforeFall2015"
	StudentsEnteringFall2015OrLater StudentGroup = "Fall2015OrLater"
)

// GeneralEducationRequirement is one part of a general education program,
// such as Core Literacies, and the categories it is made of.
type GeneralEducationRequirement struct {
	Prefix       string                     `json:"prefix"`
	Name         string                     `json:"name"`
	StudentGroup StudentGroup               `json:"student_group"`
	Categories   []GeneralEducationCategory `json:"categories"`
}

// GeneralEducationCategory is a set of courses that satisfy part of a
// requirement, such as "Category A: The Arts".
type GeneralEducationCategory struct {
	Prefix string `json:"prefix"`
	Name   string `json:"name"`
}

type generalEducationResponse struct {
	GEGroups []struct {
		GroupType    StudentGroup `json:"groupType"`
		Requirements []struct {
			Prefix     string `json:"prefix"`
			Name       string `json:"name"`
			Categories []struct {
				Prefix string `json:"prefix"`
				Name   string `json:"name"`
			} `json:"categories"`
		} `json:"requirements"`
	} `json:"geGroups"`
}

// GeneralEducationRequirements lists a term's general education requirements
// with their categories, in the order USC lists them.
func (c *Client) GeneralEducationRequirements(ctx context.Context, termCode string) ([]GeneralEducationRequirement, error) {
	if err := validateTermCode(termCode); err != nil {
		return nil, err
	}
	var wire generalEducationResponse
	if err := c.get(ctx, "GE/TermCode", url.Values{"termCode": {termCode}}, &wire); err != nil {
		return nil, err
	}

	requirements := []GeneralEducationRequirement{}
	for _, group := range wire.GEGroups {
		for _, wireRequirement := range group.Requirements {
			requirement := GeneralEducationRequirement{
				Prefix:       wireRequirement.Prefix,
				Name:         wireRequirement.Name,
				StudentGroup: group.GroupType,
				Categories:   make([]GeneralEducationCategory, len(wireRequirement.Categories)),
			}
			for index, wireCategory := range wireRequirement.Categories {
				requirement.Categories[index] = GeneralEducationCategory(wireCategory)
			}
			requirements = append(requirements, requirement)
		}
	}
	return requirements, nil
}

// GeneralEducationCourses returns every course in a term that satisfies a
// general education category, with all sections. USC only answers when the
// category is named together with its requirement.
func (c *Client) GeneralEducationCourses(ctx context.Context, termCode, requirementPrefix, categoryPrefix string) ([]Course, error) {
	if err := validateTermCode(termCode); err != nil {
		return nil, err
	}
	if requirementPrefix == "" || categoryPrefix == "" {
		return nil, fmt.Errorf("a requirement prefix and a category prefix are required")
	}

	var wire struct {
		Courses []courseResponse `json:"courses"`
	}
	query := url.Values{
		"termCode":            {termCode},
		"geRequirementPrefix": {strings.ToUpper(requirementPrefix)},
		"categoryPrefix":      {strings.ToUpper(categoryPrefix)},
	}
	if err := c.get(ctx, "Courses/GeCoursesByTerm", query, &wire); err != nil {
		return nil, err
	}
	return normalizeCourses(wire.Courses)
}

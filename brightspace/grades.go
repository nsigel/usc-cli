package brightspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Grades is the normalized gradebook for a course.
type Grades struct {
	CourseID int     `json:"course_id"`
	Grades   []Grade `json:"grades"`
}

// Grade joins a grade item with the current user's value for that item.
type Grade struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	GradeType        string   `json:"grade_type"`
	MaxPoints        *float64 `json:"max_points"`
	Weight           *float64 `json:"weight"`
	IsBonus          bool     `json:"is_bonus"`
	ExcludeFromFinal bool     `json:"exclude_from_final"`
	Score            *float64 `json:"score"`
	ScoreMax         *float64 `json:"score_max"`
	DisplayedGrade   *string  `json:"displayed_grade"`
	Feedback         *string  `json:"feedback"`
	LastModified     *string  `json:"last_modified"`
}

// Grades returns grade items joined with the current user's grade values.
func (c *Client) Grades(ctx context.Context, courseID int) (Grades, error) {
	var items []gradeItem
	itemsPath := fmt.Sprintf("/d2l/api/le/%s/%d/grades/", leVersion, courseID)
	if err := c.get(ctx, itemsPath, nil, fmt.Sprintf("grades/items(%d)", courseID), &items); err != nil {
		return Grades{}, err
	}
	var values []gradeValue
	valuesPath := fmt.Sprintf("/d2l/api/le/%s/%d/grades/values/myGradeValues/", leVersion, courseID)
	if err := c.get(ctx, valuesPath, nil, fmt.Sprintf("grades/values(%d)", courseID), &values); err != nil {
		return Grades{}, err
	}
	byID := make(map[string]gradeValue, len(values))
	for _, value := range values {
		byID[value.ID.String()] = value
	}
	grades := make([]Grade, 0, len(items))
	for _, item := range items {
		value := byID[item.ID.String()]
		var feedback *string
		if strings.TrimSpace(value.Comments.Text) != "" {
			feedback = stringPointer(strings.TrimSpace(value.Comments.Text))
		}
		grades = append(grades, Grade{
			ID: item.ID.String(), Name: item.Name, GradeType: item.GradeType,
			MaxPoints: item.MaxPoints, Weight: item.Weight, IsBonus: item.IsBonus,
			ExcludeFromFinal: item.ExcludeFromFinal, Score: value.PointsNumerator,
			ScoreMax: value.PointsDenominator, DisplayedGrade: value.DisplayedGrade,
			Feedback: feedback, LastModified: value.LastModified,
		})
	}
	return Grades{CourseID: courseID, Grades: grades}, nil
}

// Graded returns only the items that have a score.
func (g Grades) Graded() Grades {
	graded := Grades{CourseID: g.CourseID, Grades: []Grade{}}
	for _, grade := range g.Grades {
		if grade.Score != nil {
			graded.Grades = append(graded.Grades, grade)
		}
	}
	return graded
}

// identifier is a grade object ID, which Brightspace sends as a number or a string.
type identifier json.RawMessage

func (id *identifier) UnmarshalJSON(data []byte) error {
	*id = append((*id)[:0], data...)
	return nil
}

func (id identifier) String() string {
	var value string
	if json.Unmarshal(id, &value) == nil {
		return value
	}
	return string(id)
}

type gradeItem struct {
	ID               identifier `json:"Id"`
	Name             string     `json:"Name"`
	GradeType        string     `json:"GradeType"`
	MaxPoints        *float64   `json:"MaxPoints"`
	Weight           *float64   `json:"Weight"`
	IsBonus          bool       `json:"IsBonus"`
	ExcludeFromFinal bool       `json:"ExcludeFromFinalGradeCalculation"`
}

type gradeValue struct {
	ID                identifier `json:"GradeObjectIdentifier"`
	PointsNumerator   *float64   `json:"PointsNumerator"`
	PointsDenominator *float64   `json:"PointsDenominator"`
	DisplayedGrade    *string    `json:"DisplayedGrade"`
	Comments          richText   `json:"Comments"`
	LastModified      *string    `json:"LastModified"`
}

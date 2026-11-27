package libcal

import (
	"fmt"
)

type Category string

const (
	CategoryRooms  Category = "rooms"
	CategoryPods   Category = "pods"
	CategoryAll    Category = "all"
	CategoryFirst  Category = "lvl1"
	CategorySecond Category = "lvl2"
	CategoryThird  Category = "lvl3"
)

type CategoryInfo struct {
	ID   int      `json:"id"`
	Name string   `json:"name"`
	Slug Category `json:"slug"`
	Kind string   `json:"kind"`
}

type categoryPage struct {
	CategoryInfo
	Path string
}

var leaveyCategories = []categoryPage{
	{CategoryInfo: CategoryInfo{ID: 8441, Name: "Leavey Group Study Rooms (1st Floor)", Slug: CategoryFirst, Kind: "room"}, Path: "/reserve/lvl1"},
	{CategoryInfo: CategoryInfo{ID: 4889, Name: "Leavey Group Study Rooms (2nd Floor)", Slug: CategorySecond, Kind: "room"}, Path: "/reserve/lvl2"},
	{CategoryInfo: CategoryInfo{ID: 4898, Name: "Leavey Group Study Rooms (3rd Floor)", Slug: CategoryThird, Kind: "room"}, Path: "/reserve/lvl3"},
	{CategoryInfo: CategoryInfo{ID: 49218, Name: "Leavey Study Pods", Slug: CategoryPods, Kind: "pod"}, Path: "/reserve/lvl-pods"},
}

func Categories() []CategoryInfo {
	result := make([]CategoryInfo, len(leaveyCategories))
	for i, category := range leaveyCategories {
		result[i] = category.CategoryInfo
	}
	return result
}

func selectCategories(selector Category) ([]categoryPage, error) {
	var selected []categoryPage
	for _, category := range leaveyCategories {
		if selector == "" || selector == CategoryAll || selector == category.Slug ||
			selector == CategoryRooms && category.Kind == "room" {
			selected = append(selected, category)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("unknown Leavey category %q (use rooms, pods, all, lvl1, lvl2, or lvl3)", selector)
	}
	return selected, nil
}

func categoryOf(space Space) (categoryPage, error) {
	for _, category := range leaveyCategories {
		if category.ID == space.CategoryID {
			return category, nil
		}
	}
	return categoryPage{}, fmt.Errorf("unsupported Leavey reservation category %d", space.CategoryID)
}

func (c categoryPage) returnPath() string {
	for index, candidate := range leaveyCategories {
		if candidate.ID == c.ID {
			return fmt.Sprintf("%s?c=%d", c.Path, index+1)
		}
	}
	return c.Path
}

const defaultReferer = baseURL + "/reserve/lvl2"

func refererFor(space Space) string {
	category, err := categoryOf(space)
	if err != nil {
		return defaultReferer
	}
	return baseURL + category.returnPath()
}

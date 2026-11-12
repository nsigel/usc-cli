package libcal

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	http "github.com/saucesteals/fhttp"
)

// Space is a reservable room or pod in Leavey Library.
type Space struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Capacity     int      `json:"capacity"`
	Category     string   `json:"category"`
	CategoryID   int      `json:"category_id"`
	CategorySlug Category `json:"category_slug"`
	Kind         string   `json:"kind"`
	URL          string   `json:"url"`
}

// Spaces lists reservable Leavey spaces. A blank selector or "all" includes
// group study rooms and pods; "rooms" excludes pods.
func (c *Client) Spaces(ctx context.Context, selector Category) ([]Space, error) {
	selected, err := selectCategories(selector)
	if err != nil {
		return nil, err
	}
	var spaces []Space
	for _, category := range selected {
		items, err := c.spacesIn(ctx, category)
		if err != nil {
			return nil, err
		}
		spaces = append(spaces, items...)
	}
	sort.Slice(spaces, func(i, j int) bool {
		if spaces[i].CategoryID != spaces[j].CategoryID {
			return spaces[i].CategoryID < spaces[j].CategoryID
		}
		return spaces[i].Name < spaces[j].Name
	})
	return spaces, nil
}

// Space returns one Leavey space by ID.
func (c *Client) Space(ctx context.Context, id int) (Space, error) {
	spaces, err := c.Spaces(ctx, CategoryAll)
	if err != nil {
		return Space{}, err
	}
	for _, space := range spaces {
		if space.ID == id {
			return space, nil
		}
	}
	return Space{}, fmt.Errorf("Leavey space %d was not found", id)
}

// A category page declares each space as a JavaScript object literal:
//
//	resources.push({
//	    eid: 31391,
//	    title: "LVL 201",
//	    ...
//	});
var (
	resourceBlock    = regexp.MustCompile(`(?s)resources\.push\(\{(.*?)\}\);`)
	resourceProperty = regexp.MustCompile(`(?m)^\s*(\w+):\s*("(?:\\.|[^"\\])*"|-?\d+)`)
)

func (c *Client) spacesIn(ctx context.Context, category categoryPage) ([]Space, error) {
	data, err := c.request(ctx, http.MethodGet, category.Path, nil, "text/html,application/xhtml+xml", "")
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", category.Name, err)
	}
	blocks := resourceBlock.FindAllSubmatch(data, -1)
	if len(blocks) == 0 {
		return nil, fmt.Errorf("parse %s: no reservable rooms found", category.Name)
	}
	spaces := make([]Space, len(blocks))
	for index, block := range blocks {
		space, err := parseSpace(string(block[1]))
		if err != nil {
			return nil, fmt.Errorf("parse %s room: %w", category.Name, err)
		}
		space.Category, space.CategoryID, space.CategorySlug, space.Kind = category.Name, category.ID, category.Slug, category.Kind
		spaces[index] = space
	}
	return spaces, nil
}

func parseSpace(block string) (Space, error) {
	properties := make(map[string]string)
	for _, match := range resourceProperty.FindAllStringSubmatch(block, -1) {
		if _, seen := properties[match[1]]; !seen {
			properties[match[1]] = match[2]
		}
	}
	id, idErr := strconv.Atoi(properties["eid"])
	capacity, capacityErr := strconv.Atoi(properties["capacity"])
	name, nameErr := strconv.Unquote(properties["title"])
	path, pathErr := strconv.Unquote(properties["url"])
	if err := errors.Join(idErr, capacityErr, nameErr, pathErr); err != nil {
		return Space{}, fmt.Errorf("eid, capacity, title, or url is missing or malformed: %w", err)
	}
	return Space{ID: id, Name: name, Capacity: capacity, URL: baseURL + path}, nil
}

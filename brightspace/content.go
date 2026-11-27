package brightspace

import (
	"context"
	"fmt"
)

type TableOfContents struct {
	CourseID int      `json:"course_id"`
	Modules  []Module `json:"modules"`
}

type Module struct {
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Description  *string  `json:"description"`
	LastModified *string  `json:"last_modified"`
	Topics       []Topic  `json:"topics"`
	Modules      []Module `json:"modules"`
}

type Topic struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Type         int     `json:"type"`
	URL          string  `json:"url"`
	DueDate      *string `json:"due_date"`
	LastModified *string `json:"last_modified"`

	Module string `json:"module,omitempty"`
}

func (c *Client) Content(ctx context.Context, courseID int) (TableOfContents, error) {
	var response struct {
		Modules []contentModule `json:"Modules"`
	}
	path := fmt.Sprintf("/d2l/api/le/%s/%d/content/toc", leVersion, courseID)
	if err := c.get(ctx, path, nil, fmt.Sprintf("toc(%d)", courseID), &response); err != nil {
		return TableOfContents{}, err
	}
	return TableOfContents{CourseID: courseID, Modules: normalizeModules(response.Modules)}, nil
}

func (t TableOfContents) Flatten() []Topic {
	return flattenTopics(t.Modules, "")
}

func flattenTopics(modules []Module, parent string) []Topic {
	topics := []Topic{}
	for _, module := range modules {
		path := module.Title
		if parent != "" {
			path = parent + " > " + path
		}
		for _, topic := range module.Topics {
			topic.Module = path
			topics = append(topics, topic)
		}
		topics = append(topics, flattenTopics(module.Modules, path)...)
	}
	return topics
}

func (c *Client) Topic(ctx context.Context, courseID, topicID int) (Topic, error) {
	toc, err := c.Content(ctx, courseID)
	if err != nil {
		return Topic{}, err
	}
	for _, topic := range toc.Flatten() {
		if topic.ID == topicID {
			return topic, nil
		}
	}
	return Topic{}, fmt.Errorf("topic %d was not found in course %d", topicID, courseID)
}

type contentModule struct {
	ID           int             `json:"ModuleId"`
	Title        string          `json:"Title"`
	Description  *richText       `json:"Description"`
	LastModified *string         `json:"LastModifiedDate"`
	Topics       []contentTopic  `json:"Topics"`
	Modules      []contentModule `json:"Modules"`
}

type contentTopic struct {
	ID           int     `json:"TopicId"`
	Title        string  `json:"Title"`
	Type         int     `json:"Type"`
	URL          string  `json:"Url"`
	DueDate      *string `json:"DueDate"`
	LastModified *string `json:"LastModifiedDate"`
}

func normalizeModules(modules []contentModule) []Module {
	result := make([]Module, len(modules))
	for index, module := range modules {
		result[index] = Module{
			ID: module.ID, Title: module.Title, LastModified: module.LastModified,
			Topics: make([]Topic, len(module.Topics)), Modules: normalizeModules(module.Modules),
		}
		if module.Description != nil {
			result[index].Description = stringPointer(module.Description.Text)
		}
		for topicIndex, topic := range module.Topics {
			result[index].Topics[topicIndex] = Topic{ID: topic.ID, Title: topic.Title, Type: topic.Type, URL: topic.URL, DueDate: topic.DueDate, LastModified: topic.LastModified}
		}
	}
	return result
}

package handshake

import "strings"

type categoryWire struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	BehaviorIdentifier string `json:"behaviorIdentifier"`
}

type imageWire struct {
	URL string `json:"url"`
}

type employerWire struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Logo     imageWire `json:"logo"`
	Industry struct {
		Name string `json:"name"`
	} `json:"industry"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
}

type contactWire struct {
	ID           string `json:"id"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Title        string `json:"title"`
	EmailAddress string `json:"emailAddress"`
	User         struct {
		Photo string `json:"userPhotoUrl"`
	} `json:"user"`
}

type hostWire struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Typename string    `json:"__typename"`
	Name     string    `json:"name"`
	Logo     imageWire `json:"logo"`
	LogoURL  string    `json:"logoUrl"`
}

type abstractionWire struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	Favorited           bool           `json:"favorited"`
	StartDate           string         `json:"startDate"`
	EndDate             string         `json:"endDate"`
	Registered          bool           `json:"registered"`
	Medium              string         `json:"medium"`
	CareerFairMedium    string         `json:"careerFairMedium"`
	Employers           []employerWire `json:"employers"`
	Categories          []categoryWire `json:"categories"`
	Host                hostWire       `json:"host"`
	SameSchoolHost      bool           `json:"sameSchoolHost"`
	SameSchoolEvent     bool           `json:"sameSchoolEvent"`
	RegistrationEnd     *string        `json:"studentRegistrationEnd"`
	RegistrationStart   *string        `json:"studentRegistrationStart"`
	RegisteredAttendees int            `json:"registeredAttendeesCount"`
}

type abstractionEdgeWire struct {
	Cursor string          `json:"cursor"`
	Node   abstractionWire `json:"node"`
}

type abstractionConnectionWire struct {
	PageInfo struct {
		EndCursor   string `json:"endCursor"`
		HasNextPage bool   `json:"hasNextPage"`
	} `json:"pageInfo"`
	Edges []abstractionEdgeWire `json:"edges"`
}

func normalizeCategory(category categoryWire) Category {
	return Category{ID: category.ID, Name: category.Name, Slug: slug(category.Name)}
}

func normalizeCategories(categories []categoryWire) []Category {
	result := make([]Category, len(categories))
	for index, category := range categories {
		result[index] = normalizeCategory(category)
	}
	return result
}

func normalizeEmployers(employers []employerWire) []Employer {
	result := make([]Employer, len(employers))
	for index, employer := range employers {
		result[index] = Employer{ID: employer.ID, Name: employer.Name, Logo: employer.Logo.URL, Industry: employer.Industry.Name, Location: employer.Location.Name}
	}
	return result
}

func normalizeContacts(contacts []contactWire) []Contact {
	result := make([]Contact, len(contacts))
	for index, contact := range contacts {
		result[index] = Contact{ID: contact.ID, Name: strings.TrimSpace(contact.FirstName + " " + contact.LastName), Title: contact.Title, Email: contact.EmailAddress, Photo: contact.User.Photo}
	}
	return result
}

func normalizeHost(host hostWire) Organization {
	logo := host.LogoURL
	if logo == "" {
		logo = host.Logo.URL
	}
	hostType := host.Type
	if hostType == "" {
		hostType = host.Typename
	}
	return Organization{ID: host.ID, Name: host.Name, Type: strings.ToLower(hostType), Logo: logo}
}

func normalizeEventSummary(event abstractionWire) EventSummary {
	return EventSummary{
		ID: event.ID, Name: event.Name, StartDate: event.StartDate, EndDate: event.EndDate,
		Medium: strings.ToLower(event.Medium), Categories: normalizeCategories(event.Categories),
		Host: normalizeHost(event.Host), Employers: normalizeEmployers(event.Employers),
		Favorited: event.Favorited, Registered: event.Registered, SameSchoolHost: event.SameSchoolHost,
		SameSchoolEvent: event.SameSchoolEvent, RegistrationStart: event.RegistrationStart,
		RegistrationEnd: event.RegistrationEnd, RegisteredAttendees: event.RegisteredAttendees,
		URL: baseURL + "/stu/events/" + event.ID,
	}
}

func normalizeCareerFairSummary(fair abstractionWire) CareerFairSummary {
	medium := fair.CareerFairMedium
	if medium == "" {
		medium = fair.Medium
	}
	return CareerFairSummary{
		ID: fair.ID, Name: fair.Name, StartDate: fair.StartDate, EndDate: fair.EndDate,
		Medium: strings.ToLower(medium), Categories: normalizeCategories(fair.Categories),
		Host: normalizeHost(fair.Host), Employers: normalizeEmployers(fair.Employers),
		Favorited: fair.Favorited, Registered: fair.Registered, SameSchoolHost: fair.SameSchoolHost,
		RegistrationStart: fair.RegistrationStart, RegistrationEnd: fair.RegistrationEnd,
		RegisteredAttendees: fair.RegisteredAttendees, URL: baseURL + "/stu/career_fairs/" + fair.ID,
	}
}

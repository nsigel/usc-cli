package handshake

const categoriesQuery = `query GetEventAbstractionCategories {
  eventAbstractionCategories { id name behaviorIdentifier }
}`

const eventAbstractionsQuery = `query GetEventAbstractions($params: EventAbstractionSearchInput, $first: Int, $after: String, $maxEmployersCount: Int) {
  eventAbstractions(params: $params, first: $first, after: $after) {
    pageInfo { endCursor hasNextPage }
    edges {
      cursor
      node {
        id
        name
        type
        favorited
        startDate
        endDate
        registered
        medium
        employers(limit: $maxEmployersCount) {
          id
          name
          logo { url(size: "small") }
          industry { name }
          location { name }
        }
        categories { id name behaviorIdentifier }
        ... on Event {
          host { logoUrl name }
          sameSchoolHost
          sameSchoolEvent
          studentRegistrationEnd
          studentRegistrationStart
          registeredAttendeesCount
        }
        ... on CareerFair {
          host { logoUrl name }
          careerFairMedium
          sameSchoolHost
          studentRegistrationEnd
          studentRegistrationStart
          registeredAttendeesCount
        }
      }
    }
  }
}`

const eventQuery = `query GetEvent($id: ID!) {
  event(id: $id) {
    id
    name
    description
    startDate
    endDate
    eventType { name behaviorIdentifier }
    eventMediumType
    externalLink
    externalRegistration
    legalDisclaimer
    studentRegistrationEnd
    studentRegistrationStart
    virtualLink
    availableSpace
    waitlistEnabled
    waitlistedCount
    registeredAttendeesCount
    timeZone
    image { url }
    room { name building { name location { name } } }
    location { name }
    owner {
      ... on Employer { id name logo { url } }
      ... on School { id name logo { url } }
      __typename
    }
    host { id type name logo { url } }
    attendee { registered registering waitlisted checkedIn }
    categories { id name behaviorIdentifier }
    contacts { id firstName lastName title emailAddress user { userPhotoUrl } }
    employers {
      id
      name
      logo { url(size: "small") }
      industry { name }
      location { name }
    }
  }
}`

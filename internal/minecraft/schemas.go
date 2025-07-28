package minecraft

type version struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type player struct {
	Name string `json:"name"`
	Uuid string `json:"id"`
}

type players struct {
	Max    int      `json:"max"`
	Online int      `json:"online"`
	Sample []player `json:"sample"`
}

type description struct {
	Text string `json:"text"`
}

type status struct {
	Version            version     `json:"version"`
	Players            players     `json:"players"`
	Description        description `json:"description"`
	Favicon            string      `json:"favicon"`
	EnforcesSecureChat bool        `json:"enforcesSecureChat"`
}

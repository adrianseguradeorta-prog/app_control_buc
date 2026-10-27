package core

import "time"

const SchemaVersion = 4

type Config struct {
	AllowedHosts   []string `json:"allowed_hosts,omitempty"`
	AppName        string   `json:"app_name"`
	UnitName       string   `json:"unit_name"`
	PreferredPort  int      `json:"preferred_port"`
	BindAddress    string   `json:"bind_address"`
	ReportLocation string   `json:"report_location"`
	Sign1Role      string   `json:"sign1_role"`
	Sign1Name      string   `json:"sign1_name"`
	Sign2Role      string   `json:"sign2_role"`
	Sign2Name      string   `json:"sign2_name"`
}

type User struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	PasswordSalt   string `json:"password_salt"`
	PasswordHash   string `json:"password_hash"`
	HashIterations int    `json:"hash_iterations,omitempty"`
	Status         string `json:"status"`
	IsAdmin        bool   `json:"is_admin"`
	CreatedAt      string `json:"created_at"`
	ApprovedAt     string `json:"approved_at,omitempty"`
}

type Diver struct {
	ID            string `json:"id"`
	Rank          string `json:"rank"`
	Name          string `json:"name"`
	EntryDate     string `json:"entry_date"`
	QualBasic     bool   `json:"qual_basic"`
	QualSDSAir    bool   `json:"qual_sds_air"`
	QualSDSHeliox bool   `json:"qual_sds_heliox"`
	Active        bool   `json:"active"`
	Notes         string `json:"notes"`
	Photo         string `json:"photo,omitempty"`
}

type Dive struct {
	ID            int     `json:"id"`
	SourceRecord  string  `json:"source_record,omitempty"`
	DiverID       string  `json:"diver_id"`
	Date          string  `json:"date"`
	Qualification string  `json:"qualification"`
	BreathingMode string  `json:"breathing_mode,omitempty"`
	Gas           string  `json:"gas"`
	SurfaceSupply bool    `json:"surface_supply"`
	MaxDepth      float64 `json:"max_depth"`
	BottomTime    int     `json:"bottom_time"`
	DiveType      string  `json:"dive_type"`
	IBCode        string  `json:"ib_code"`
	Purpose       string  `json:"purpose,omitempty"`
	Notes         string  `json:"notes"`
	Valid         bool    `json:"valid"`
	CreatedAt     string  `json:"created_at,omitempty"`
}

type ReportTemplate struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Title             string `json:"title"`
	IntroTemplate     string `json:"intro_template"`
	ColName           string `json:"col_name"`
	ColDate           string `json:"col_date"`
	ColEquipment      string `json:"col_equipment"`
	ColGas            string `json:"col_gas"`
	ColCode           string `json:"col_code"`
	LocationTemplate  string `json:"location_template"`
	Sign1Role         string `json:"sign1_role"`
	Sign1Name         string `json:"sign1_name"`
	Sign2Role         string `json:"sign2_role"`
	Sign2Name         string `json:"sign2_name"`
	FontFamily        string `json:"font_family"`
	BodyFontSize      int    `json:"body_font_size"`
	TableFontSize     int    `json:"table_font_size"`
	SignatureTopSpace int    `json:"signature_top_space"`
	CreatedAt         string `json:"created_at"`
}

type Store struct {
	SchemaVersion           int              `json:"schema_version"`
	NextDiveID              int              `json:"next_dive_id"`
	Users                   []User           `json:"users"`
	Divers                  []Diver          `json:"divers"`
	Dives                   []Dive           `json:"dives"`
	ReportTemplates         []ReportTemplate `json:"report_templates"`
	DefaultReportTemplateID string           `json:"default_report_template_id"`
}

type QualState struct {
	Code       string
	Label      string
	Status     string
	LastDive   string
	Count12m   int
	Required   int
	SpecificOK bool
	Detail     string
	Deadline   string
	Recovery   string
	Dives      []Dive
}

func ParseDate(s string) (time.Time, error) { return time.Parse("2006-01-02", s) }

package domain

// Organization is the studio brand that owns every branch.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ActiveStatus is the shared ACTIVE/INACTIVE flag used by branches and coaches.
type ActiveStatus string

const (
	StatusActive   ActiveStatus = "ACTIVE"
	StatusInactive ActiveStatus = "INACTIVE"
)

// Branch is one physical location. RulesOverride lets a branch differ from the
// organization defaults without forking configuration.
type Branch struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Address        string         `json:"address"`
	Timezone       string         `json:"timezone"`
	OperatingHours string         `json:"operatingHours"`
	Status         ActiveStatus   `json:"status"`
	ManagerName    *string        `json:"managerName"`
	RulesOverride  *RulesOverride `json:"rulesOverride"`
}

// GateStatus reports whether a turnstile is reachable from the server. An
// OFFLINE gate still admits members via the local cache and syncs later.
type GateStatus string

const (
	GateOnline  GateStatus = "ONLINE"
	GateOffline GateStatus = "OFFLINE"
)

// Gate is a turnstile or door controller at a branch.
type Gate struct {
	ID       string     `json:"id"`
	BranchID string     `json:"branchId"`
	Name     string     `json:"name"`
	Status   GateStatus `json:"status"`
}

// Coach delivers class sessions and is paid through the incentive module.
type Coach struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Bio            string       `json:"bio"`
	Specialization string       `json:"specialization"`
	BranchID       string       `json:"branchId"`
	Status         ActiveStatus `json:"status"`
}

// AdminUser is a staff account. A nil BranchID means the user is not scoped to
// a single branch (HQ level).
type AdminUser struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     AdminRole `json:"role"`
	BranchID *string   `json:"branchId"`
}

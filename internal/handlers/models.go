package handlers

// CreateArchive
type createArchiveReq struct {
	URLs []string `json:"urls"`
}

type createArchiveResp struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Files      []string `json:"files"`
	Errors     []string `json:"errors,omitempty"`
	CreatedAt  string   `json:"created_at"`
	ArchiveURL string   `json:"archive_url,omitempty"`
}

// CreateEmptyArchive
type createEmptyArchiveResp struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// AddFile
type addFileReq struct {
	URL string `json:"url"`
}

type addFileResp struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// GetArchiveStatus
type getArchiveStatusResp struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Files      []string `json:"files"`
	Errors     []string `json:"errors,omitempty"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	ArchiveURL string   `json:"archive_url,omitempty"`
}

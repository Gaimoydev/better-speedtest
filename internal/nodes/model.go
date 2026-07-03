package nodes

type Node struct {
	Name     string  `json:"name"`
	Source   string  `json:"source"`
	Carrier  string  `json:"carrier,omitempty"`
	Province string  `json:"province,omitempty"`
	City     string  `json:"city,omitempty"`
	Region   string  `json:"region,omitempty"`
	HostIP   string  `json:"host_ip,omitempty"`
	Port     string  `json:"port,omitempty"`
	DLURL    string  `json:"dl_url,omitempty"`
	ULURL    string  `json:"ul_url,omitempty"`
	UA       string  `json:"ua,omitempty"`
	Enhanced bool    `json:"enhanced,omitempty"`
	DistKM   float64 `json:"dist_km,omitempty"`
}

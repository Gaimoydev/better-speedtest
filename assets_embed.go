package betterspeedtest

import _ "embed"

//go:embed assets/config.default.json
var DefaultConfig []byte

//go:embed assets/mcc-mnc.csv
var MccMnc []byte

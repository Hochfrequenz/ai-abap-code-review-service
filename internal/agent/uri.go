package agent

import (
	"strings"

	"github.com/Hochfrequenz/adtler/adt"
)

// ObjectURI maps a TransportObject to its ADT URI path.
// Returns "" for unsupported types (FUGR, DTEL, etc.) — the agent
// prompt instructs Claude to skip objects with empty URIs.
func ObjectURI(obj adt.TransportObject) string {
	name := strings.ToLower(obj.Name)
	switch obj.Type {
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + name
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + name
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + name
	default:
		return ""
	}
}

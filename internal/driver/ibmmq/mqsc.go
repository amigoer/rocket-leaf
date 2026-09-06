package ibmmq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

/*
 * MQSC over the administrative REST API.
 *
 * The REST API describes queues, channels and subscriptions as JSON, and that
 * is the interface this driver prefers wherever it exists. It does not exist
 * for everything: there is no topic resource at all, no listener resource, no
 * channel status resource, and the subscription resource is read only. What
 * covers the rest is one endpoint - action/qmgr/{qmgr}/mqsc - which runs an
 * MQSC command and answers with its output.
 *
 * It has two request shapes and only one is usable here. "runCommand" takes a
 * command as a string and answers with the lines runmqsc would have printed,
 * which means parsing fixed-width AMQ8414I text. "runCommandJSON" takes the
 * verb, the object type and the parameters as fields and answers with a
 * decoded map per object, so the driver reads named values rather than
 * scraping a report. Everything here uses the second.
 */

// mqscRequest is one runCommandJSON call.
type mqscRequest struct {
	Type      string `json:"type"`
	Command   string `json:"command"`
	Qualifier string `json:"qualifier"`
	Name      string `json:"name,omitempty"`

	// Parameters are the ones the command takes; ResponseParameters are the
	// attributes a display should return. A display with neither returns the
	// object's name and nothing else, which is why every read here names what
	// it wants.
	Parameters         map[string]any `json:"parameters,omitempty"`
	ResponseParameters []string       `json:"responseParameters,omitempty"`
}

// mqscResult is one object, or one command's outcome.
type mqscResult struct {
	CompletionCode int                        `json:"completionCode"`
	ReasonCode     int                        `json:"reasonCode"`
	Parameters     map[string]json.RawMessage `json:"parameters"`
	Message        []string                   `json:"message"`
	Text           []string                   `json:"text"`
}

type mqscResponse struct {
	CommandResponse []mqscResult `json:"commandResponse"`
	OverallReason   int          `json:"overallReasonCode"`
	OverallComplete int          `json:"overallCompletionCode"`
	Error           []struct {
		Message string `json:"message"`
	} `json:"error"`
}

// mqscNotFound is the reason code the queue manager returns when a display
// matched no object. It is not a failure: an empty result is how "there are
// none" is spelled, and a queue manager with no channel running answers every
// status display this way.
const mqscNotFound = 3065

// display runs one MQSC display and returns a decoded map per object.
//
// A display that matched nothing comes back as an empty slice rather than an
// error, because the two are indistinguishable to a caller that only wants a
// list and very distinguishable to one that would otherwise fail a page.
func (c *Conn) display(ctx context.Context, qualifier, name string, attributes ...string) ([]map[string]json.RawMessage, error) {
	response, err := c.runMQSC(ctx, mqscRequest{
		Type:               "runCommandJSON",
		Command:            "display",
		Qualifier:          qualifier,
		Name:               name,
		ResponseParameters: attributes,
	})
	if err != nil {
		return nil, err
	}

	objects := make([]map[string]json.RawMessage, 0, len(response.CommandResponse))
	for _, result := range response.CommandResponse {
		if result.CompletionCode != 0 {
			if result.ReasonCode == mqscNotFound {
				continue
			}
			return nil, mqscFailure(qualifier, name, result)
		}
		if result.Parameters != nil {
			objects = append(objects, result.Parameters)
		}
	}
	return objects, nil
}

// command runs one MQSC command that changes something.
//
// Unlike display it fails on any non-zero completion code, including the
// "not found" one: a delete that matched nothing did not do what was asked.
func (c *Conn) command(ctx context.Context, verb, qualifier, name string, parameters map[string]any) error {
	response, err := c.runMQSC(ctx, mqscRequest{
		Type:       "runCommandJSON",
		Command:    verb,
		Qualifier:  qualifier,
		Name:       name,
		Parameters: parameters,
	})
	if err != nil {
		return err
	}
	for _, result := range response.CommandResponse {
		if result.CompletionCode != 0 {
			return mqscFailure(qualifier, name, result)
		}
	}
	return nil
}

// runMQSC posts one command and decodes the envelope.
func (c *Conn) runMQSC(ctx context.Context, request mqscRequest) (*mqscResponse, error) {
	if err := c.live(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	raw, _, err := c.rest.do(ctx, "POST",
		c.rest.base+"/ibmmq/rest/v1/admin/action/qmgr/"+c.qmgr+"/mqsc",
		c.rest.adminUP, "application/json", payload, nil)
	if err != nil {
		return nil, err
	}

	var response mqscResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("the mqsc endpoint answered something that is not json: %w", err)
	}
	if len(response.Error) > 0 {
		return nil, fmt.Errorf("mqsc %s %s: %s", request.Command, request.Qualifier, response.Error[0].Message)
	}
	return &response, nil
}

// mqscFailure names what the queue manager said, which is an AMQ message
// rather than an HTTP status.
func mqscFailure(qualifier, name string, result mqscResult) error {
	lines := append(append([]string{}, result.Message...), result.Text...)
	detail := strings.TrimSpace(strings.Join(lines, " "))
	if detail == "" {
		detail = fmt.Sprintf("completion %d, reason %d", result.CompletionCode, result.ReasonCode)
	}
	return fmt.Errorf("mqsc %s(%s): %s", qualifier, name, detail)
}

// mqscString reads one attribute as text.
//
// MQSC values arrive typed - a name is a string, a depth is a number, and a
// compression list is an array - so every read goes through a decoder that
// says which shape it expected. Trailing spaces are cut because MQ pads
// fixed-width fields and a padded name compares unequal to the same name.
func mqscString(object map[string]json.RawMessage, key string) string {
	raw, ok := object[key]
	if !ok {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimRight(text, " ")
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}
	return ""
}

// mqscInt reads one attribute as a whole number, and reports whether it was
// there at all - which is not the same as being zero.
func mqscInt(object map[string]json.RawMessage, key string) (int64, bool) {
	raw, ok := object[key]
	if !ok {
		return 0, false
	}
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

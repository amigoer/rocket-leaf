package model

import "testing"

func TestRequiresEndpointsReadsTheForm(t *testing.T) {
	tests := []struct {
		name string
		form []FormField
		want bool
	}{
		{
			name: "required endpoint field",
			form: []FormField{{Key: "endpoints", Target: TargetEndpoints, Required: true}},
			want: true,
		},
		{
			name: "no endpoint field",
			form: []FormField{
				{Key: "region", Target: TargetOption, Required: true},
				{Key: "accessKeyId", Target: TargetSecret, Required: true},
			},
			want: false,
		},
		// An optional address is a hint, not a requirement: a family that can
		// derive its own endpoint must still save without one.
		{
			name: "optional endpoint field",
			form: []FormField{{Key: "endpoints", Target: TargetEndpoints}},
			want: false,
		},
		{
			name: "empty form",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := DriverDescriptor{Kind: "fake", Form: test.form}
			if got := descriptor.RequiresEndpoints(); got != test.want {
				t.Errorf("RequiresEndpoints() = %v; want %v", got, test.want)
			}
		})
	}
}

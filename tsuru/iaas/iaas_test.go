package iaas

import (
	"testing"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

type iaasTest struct{}

func (i *iaasTest) CreateMachine(params map[string]string) (*Machine, error) {
	return &Machine{}, nil
}

func (i *iaasTest) DeleteMachine(m *Machine) error {
	return nil
}

func (s *S) TestRegister(c *check.C) {
	Register("abc", &iaasTest{})
	provider, ok := iaasProviders["abc"]
	c.Assert(ok, check.Equals, true)
	c.Assert(provider, check.FitsTypeOf, &iaasTest{})
}

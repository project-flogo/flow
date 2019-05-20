package definition

import (
	"github.com/project-flogo/core/data/expression"
)

var exprFactory expression.Factory

func SetExprFactory(factory expression.Factory) {
	exprFactory = factory
}

func GetExprFactory() expression.Factory {
	return exprFactory
}

func NewLinkExprError(msg string) *LinkExprError {
	return &LinkExprError{msg: msg}
}

// LinkExprError thrown if error is encountered evaluating an link expression
type LinkExprError struct {
	msg string
}

func (e *LinkExprError) Error() string {
	return e.msg
}

// GetExpressionLinks gets the links of the definition that are of type LtExpression
func GetExpressionLinks(def *Definition) []*Link {

	var links []*Link

	for _, link := range def.Links() {

		if link.Type() == LtExpression {
			links = append(links, link)
		}
	}

	if def.GetErrorHandler() != nil {
		for _, link := range def.GetErrorHandler().links {

			if link.Type() == LtExpression {
				links = append(links, link)
			}
		}
	}

	return links
}

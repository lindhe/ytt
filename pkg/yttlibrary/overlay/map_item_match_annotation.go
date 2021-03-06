package overlay

import (
	"fmt"
	"reflect"

	"github.com/k14s/ytt/pkg/filepos"
	"github.com/k14s/ytt/pkg/template"
	tplcore "github.com/k14s/ytt/pkg/template/core"
	"github.com/k14s/ytt/pkg/yamlmeta"
	"github.com/k14s/ytt/pkg/yamltemplate"
	"go.starlark.net/starlark"
)

type MapItemMatchAnnotation struct {
	newItem *yamlmeta.MapItem
	thread  *starlark.Thread

	matcher *starlark.Value
	expects MatchAnnotationExpectsKwarg
}

func NewMapItemMatchAnnotation(newItem *yamlmeta.MapItem,
	defaults MatchChildDefaultsAnnotation,
	thread *starlark.Thread) (MapItemMatchAnnotation, error) {

	annotation := MapItemMatchAnnotation{
		newItem: newItem,
		thread:  thread,
		expects: MatchAnnotationExpectsKwarg{thread: thread},
	}
	kwargs := template.NewAnnotations(newItem).Kwargs(AnnotationMatch)

	for _, kwarg := range kwargs {
		kwargName := string(kwarg[0].(starlark.String))
		switch kwargName {
		case MatchAnnotationKwargBy:
			annotation.matcher = &kwarg[1]
		case MatchAnnotationKwargExpects:
			annotation.expects.expects = &kwarg[1]
		case MatchAnnotationKwargMissingOK:
			annotation.expects.missingOK = &kwarg[1]
		default:
			return annotation, fmt.Errorf(
				"Unknown '%s' annotation keyword argument '%s'", AnnotationMatch, kwargName)
		}
	}

	annotation.expects.FillInDefaults(defaults)

	return annotation, nil
}

func (a MapItemMatchAnnotation) Indexes(leftMap *yamlmeta.Map) ([]int, error) {
	idxs, matches, err := a.MatchNodes(leftMap)
	if err != nil {
		return []int{}, err
	}

	return idxs, a.expects.Check(matches)
}

func (a MapItemMatchAnnotation) MatchNodes(leftMap *yamlmeta.Map) ([]int, []*filepos.Position, error) {
	if a.matcher == nil {
		var leftIdxs []int
		var matches []*filepos.Position

		for i, item := range leftMap.Items {
			if reflect.DeepEqual(item.Key, a.newItem.Key) {
				leftIdxs = append(leftIdxs, i)
				matches = append(matches, item.Position)
			}
		}
		return leftIdxs, matches, nil
	}

	switch typedVal := (*a.matcher).(type) {
	case starlark.String:
		var leftIdxs []int
		var matches []*filepos.Position

		for i, item := range leftMap.Items {
			result, err := overlayModule{}.compareByMapKey(string(typedVal), item, a.newItem)
			if err != nil {
				return nil, nil, err
			}
			if result {
				leftIdxs = append(leftIdxs, i)
				matches = append(matches, item.Position)
			}
		}

		return leftIdxs, matches, nil

	case starlark.Callable:
		var leftIdxs []int
		var matches []*filepos.Position

		for i, item := range leftMap.Items {
			matcherArgs := starlark.Tuple{
				yamltemplate.NewStarlarkFragment(item.Key),
				yamltemplate.NewStarlarkFragment(item.Value),
				yamltemplate.NewStarlarkFragment(a.newItem.Value),
			}

			// TODO check thread correctness
			result, err := starlark.Call(a.thread, *a.matcher, matcherArgs, []starlark.Tuple{})
			if err != nil {
				return nil, nil, err
			}

			resultBool, err := tplcore.NewStarlarkValue(result).AsBool()
			if err != nil {
				return nil, nil, err
			}
			if resultBool {
				leftIdxs = append(leftIdxs, i)
				matches = append(matches, item.Position)
			}
		}
		return leftIdxs, matches, nil
	default:
		return nil, nil, fmt.Errorf("Expected '%s' annotation keyword argument 'by'"+
			" to be either string (for map key) or function, but was %T", AnnotationMatch, typedVal)
	}
}

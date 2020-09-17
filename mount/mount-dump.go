package mount

import "fmt"

func printIndent(indent int) {
	for i := 0; i < indent; i++ {
		fmt.Print("  ")
	}
}

func (n *node) dump(indent int) {

	printIndent(indent)
	fmt.Printf("node (%s), type %T, object %v\n", n.completePath, n.node, n.node)

	for s, p := range n.ch {
		printIndent(indent + 1)
		fmt.Printf("going to (%s)\n", s)
		p.dump(indent + 2)
	}

}

module github.com/notrobot1/funcscan/funcweb

go 1.23

require (
	github.com/notrobot1/anti_entry_points/inputclf v0.0.0-20261122140732-c9358901fb0e
	github.com/notrobot1/funcscan/funcgraph v0.0.0
)

require (
	github.com/mattn/go-pointer v0.0.1 // indirect
	github.com/notrobot1/funcscan/relationships v0.0.0 // indirect
	github.com/tree-sitter-grammars/tree-sitter-kotlin v1.1.0 // indirect
	github.com/tree-sitter-grammars/tree-sitter-lua v0.5.0 // indirect
	github.com/tree-sitter/go-tree-sitter v0.25.0 // indirect
	github.com/tree-sitter/tree-sitter-bash v0.25.1 // indirect
	github.com/tree-sitter/tree-sitter-c v0.24.2 // indirect
	github.com/tree-sitter/tree-sitter-c-sharp v0.24.0 // indirect
	github.com/tree-sitter/tree-sitter-cpp v0.23.4 // indirect
	github.com/tree-sitter/tree-sitter-go v0.25.0 // indirect
	github.com/tree-sitter/tree-sitter-java v0.23.5 // indirect
	github.com/tree-sitter/tree-sitter-javascript v0.25.0 // indirect
	github.com/tree-sitter/tree-sitter-php v0.25.2 // indirect
	github.com/tree-sitter/tree-sitter-python v0.25.0 // indirect
	github.com/tree-sitter/tree-sitter-ruby v0.23.1 // indirect
	github.com/tree-sitter/tree-sitter-rust v0.24.2 // indirect
	github.com/tree-sitter/tree-sitter-scala v0.26.13 // indirect
	github.com/tree-sitter/tree-sitter-typescript v0.23.2 // indirect
	go.etcd.io/bbolt v1.3.9 // indirect
	golang.org/x/sys v0.4.0 // indirect
)

replace (
	github.com/notrobot1/funcscan/funcgraph => ../funcgraph
	github.com/notrobot1/funcscan/relationships => ../relationships
)

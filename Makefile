build:
	tinygo build -o model-router.wasm -scheduler=none --no-debug -target=wasi main.go
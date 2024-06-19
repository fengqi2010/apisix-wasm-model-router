build:
	tinygo build -o model_decoder.wasm -scheduler=none --no-debug -target=wasi main.go
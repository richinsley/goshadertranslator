## clone the repo
```bash
git clone https://github.com/richinsley/goshadertranslator.git
cd goshadertranslator
```

## build the docker image
```bash
# force rebuild for changes to dockerfile
docker build --no-cache .
```

## build with docker image
```bash
docker run -it \
  -e HOST_UID=$(id -u) \
  -e HOST_GID=$(id -g) \
  -v "$(pwd):/workspace" \
  pyshadertranslator-builder
```

## setup the tools and gather the angle repo
```bash
setup.sh
```

## build the shader translator wasm module
```bash
build.sh
```
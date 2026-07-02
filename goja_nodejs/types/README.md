# @joeycumines/goja_nodejs

TypeScript type definitions for the
[goja_nodejs](https://github.com/joeycumines/goja_nodejs) Node.js compatibility
layer — a fork of [dop251/goja_nodejs](https://github.com/dop251/goja_nodejs)
built on the [goja](https://github.com/joeycumines/goja) ECMAScript runtime.

Covers:

- the global `GojaNodeJS` namespace,
- the `url` / `node:url` modules (`URL`, `URLSearchParams`, `domainToASCII`, `domainToUnicode`),
- the `buffer` / `node:buffer` modules (`Buffer`, `BufferEncoding`, ...).

## Install

This package is published to the **GitHub Packages** npm registry
(`npm.pkg.github.com`). Configure npm to resolve the `@joeycumines` scope there
(a personal access token with `read:packages` is required, even for public
packages — this is a GitHub Packages limitation):

```shell
# ~/.npmrc
@joeycumines:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

Then:

```shell
npm install --save-dev @joeycumines/goja_nodejs
```

## Usage

Add the package to your `tsconfig.json` so the ambient declarations are loaded:

```json
{
  "compilerOptions": {
    "types": ["@joeycumines/goja_nodejs"]
  }
}
```

This makes the global `Buffer`, the `GojaNodeJS` namespace, and
`require('url')` / `require('buffer')` typed.

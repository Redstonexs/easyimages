#!/bin/sh
set -eu

npm_version=12.2.3
glob_version=14.0.6
minimatch_version=11.0.9
temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT

# The upstream npm tarball still bundles vulnerable glob and minimatch versions.
# Replace those packages in a temporary tarball before installing the CLI.
npm pack "npm@$npm_version" --pack-destination "$temp_dir" --ignore-scripts --silent
tar -xzf "$temp_dir/npm-$npm_version.tgz" -C "$temp_dir"
mkdir "$temp_dir/fixes"
npm install --prefix "$temp_dir/fixes" --install-strategy=nested --no-save --no-package-lock \
  --ignore-scripts --no-audit --no-fund "glob@$glob_version" "minimatch@$minimatch_version"
for package in glob minimatch; do
  rm -rf "$temp_dir/package/node_modules/$package"
  mv "$temp_dir/fixes/node_modules/$package" "$temp_dir/package/node_modules/$package"
done
npm --prefix "$temp_dir/package" pkg set "dependencies.glob=$glob_version" "dependencies.minimatch=$minimatch_version"
tar -czf "$temp_dir/npm-patched.tgz" -C "$temp_dir" package
npm install --global "$temp_dir/npm-patched.tgz" --ignore-scripts --no-audit --no-fund

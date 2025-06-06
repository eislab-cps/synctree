#!/bin/bash

# rename_project.sh
# Usage: ./rename_project.sh <new-project-name>
# Example: ./rename_project.sh eislab-cps/synctree

set -e

if [ -z "$1" ]; then
  echo "Usage: $0 <new-project-name>"
  echo "Example: $0 eislab-cps/synctree"
  exit 1
fi

OLD_NAME="eislab-cps/synctree"
NEW_NAME="$1"

OLD_MODULE="github.com/eislab-cps/synctree"
NEW_MODULE="github.com/$NEW_NAME"

OLD_PREFIX="github.com/"
NEW_PREFIX="github.com/"

echo "====================================="
echo "Renaming project from '$OLD_NAME' to '$NEW_NAME'"
echo "Updating module path from '$OLD_MODULE' to '$NEW_MODULE'"
echo "Cleaning up old prefix '$OLD_PREFIX' → '$NEW_PREFIX'"
echo "====================================="

# Determine sed options for macOS vs Linux
if [[ "$OSTYPE" == "darwin"* ]]; then
    SED_INPLACE() {
        sed -i '' -e "$1" "$2"
    }
else
    SED_INPLACE() {
        sed -i -e "$1" "$2"
    }
fi

# Step 1: Replace eislab-cps/synctree in file contents
echo "Step 1: Updating file contents (project name)..."
grep -rl "$OLD_NAME" . \
    --exclude-dir={.git,.github,vendor} \
    --exclude=rename_project.sh \
    | while IFS= read -r file; do
        echo "Updating $file"
        SED_INPLACE "s#$OLD_NAME#$NEW_NAME#g" "$file"
    done

# Step 2: Replace old import paths
echo "Step 2: Updating import paths..."
grep -rl "$OLD_MODULE" . \
    --exclude-dir={.git,.github,vendor} \
    --exclude=rename_project.sh \
    | while IFS= read -r file; do
        echo "Updating import path in $file"
        SED_INPLACE "s#$OLD_MODULE#$NEW_MODULE#g" "$file"
    done

# Step 2b: Remove old prefix github.com/
echo "Step 2b: Removing old prefix in import paths..."
grep -rl "$OLD_PREFIX" . \
    --exclude-dir={.git,.github,vendor} \
    --exclude=rename_project.sh \
    | while IFS= read -r file; do
        echo "Fixing prefix in $file"
        SED_INPLACE "s#$OLD_PREFIX#$NEW_PREFIX#g" "$file"
    done

# Step 3: Rename files and directories
echo "Step 3: Renaming files and directories..."
find . -depth -name "*$OLD_NAME*" | while IFS= read -r path; do
    new_path="$(dirname "$path")/$(basename "$path" | sed "s#$OLD_NAME#$(basename "$NEW_NAME")#g")"
    echo "Renaming '$path' -> '$new_path'"
    mv "$path" "$new_path"
done

# Step 4: Update go.mod module path
if [[ -f go.mod ]]; then
    echo "Step 4: Updating 'module' line in go.mod..."
    SED_INPLACE "s#^module .*#module $NEW_MODULE#g" go.mod
fi

# Final message
echo "====================================="
echo "✅ Renaming completed!"
echo "✅ Project name: '$NEW_NAME'"
echo "✅ Module path: '$NEW_MODULE'"
echo "====================================="
echo "Next steps:"
echo " 1️⃣ Run: go mod tidy"
echo " 2️⃣ Test build: go build ./..."
echo " 3️⃣ Commit your changes!"

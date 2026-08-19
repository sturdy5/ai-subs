#!/bin/bash

releaseType=${1:-"minor"}

# figure out which directory the script is in
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# change to the script directory
cd "$script_dir"

# do a quick build to make sure everything is working
./build.sh

# check on the latest tag name
latest_tag=$(gh release list --json tagName,isLatest --jq '.[] | select(.isLatest == true) | .tagName')
# if the latest tag is empty, set it to v0.0.0
if [ -z "$latest_tag" ]; then
  latest_tag="v0.0.0"
fi
echo "Latest tag: $latest_tag. Bumping to new $releaseType version..."

# bump the version using semver
baseVersion=${latest_tag#v}
IFS='.' read -r major minor patch <<< "$baseVersion"
case $releaseType in
  major)
    major=$((major + 1))
    minor=0
    patch=0
    ;;
  minor)
    minor=$((minor + 1))
    patch=0
    ;;
  patch)
    patch=$((patch + 1))
    ;;
  *)
    echo "Invalid release type: $releaseType. Use 'major', 'minor', or 'patch'."
    exit 1
    ;;
esac

echo "Creating new release: v$major.$minor.$patch"
git tag -a "v$major.$minor.$patch" -m "Release v$major.$minor.$patch"
git push origin "v$major.$minor.$patch"
gh release create "v$major.$minor.$patch" './ai-subs#ai-subs - linux' './ai-subs.exe#ai-subs - windows' --title "v$major.$minor.$patch" --notes "Automated release of version v$major.$minor.$patch<br/><br/>**Full Changelog:** https://github.com/sturdy5/ai-subs/compare/$latest_tag...v$major.$minor.$patch"
echo "Release v$major.$minor.$patch created successfully!"

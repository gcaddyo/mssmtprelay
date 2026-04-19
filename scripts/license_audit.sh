#!/usr/bin/env sh
set -eu

TMP_FILE="${TMPDIR:-/tmp}/relayctl_license_audit.txt"

if ! command -v go >/dev/null 2>&1; then
  echo "go command not found" >&2
  exit 1
fi

go list -m -f '{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}' all | sed '/^$/d' > "$TMP_FILE"

echo "# Third-Party License Audit"
echo
printf "%-60s %-20s %s\n" "MODULE" "LICENSE" "FILE"
printf "%-60s %-20s %s\n" "------" "-------" "----"

has_gpl=0

while IFS='|' read -r mod dir; do
  lic_file=$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) | head -n1 || true)
  if [ -z "$lic_file" ]; then
    kind="UNKNOWN"
    path="(missing)"
  else
    sample=$(awk 'NR<=80{print}' "$lic_file" | tr '[:upper:]' '[:lower:]')
    case "$sample" in
      *"gnu general public license"*|*" affero general public license"*|*" lesser general public license"*) kind="GPL-family"; has_gpl=1 ;;
      *"apache license"*) kind="Apache-2.0" ;;
      *"permission is hereby granted, free of charge"*) kind="MIT" ;;
      *"redistribution and use in source and binary forms"*) kind="BSD-style" ;;
      *"mozilla public license"*) kind="MPL" ;;
      *"isc license"*) kind="ISC" ;;
      *) kind="UNKNOWN" ;;
    esac
    path="$lic_file"
  fi
  printf "%-60s %-20s %s\n" "$mod" "$kind" "$path"
done < "$TMP_FILE"

if [ "$has_gpl" -eq 1 ]; then
  echo
  echo "RESULT: GPL-family licenses detected. Manual legal review required."
  exit 2
fi

echo
echo "RESULT: No GPL-family licenses detected in current dependency set."

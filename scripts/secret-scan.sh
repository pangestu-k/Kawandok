#!/bin/sh
set -u

echo "gitleaks not found; running basic secret pattern scan over tracked files"

files=$(git ls-files -z 2>/dev/null | tr '\0' '\n')
if [ -z "$files" ]; then
    echo "no tracked files to scan (run inside the git repository)"
    exit 1
fi

found=0
for f in $files; do
    [ -f "$f" ] || continue
    [ "$f" = "scripts/secret-scan.sh" ] && continue
    if grep -inIE \
        -e 'BEGIN (RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY' \
        -e 'AKIA[0-9A-Z]{16}' \
        -e '(aws|r2)_(secret|access)_?(key|token)[[:space:]]*[:=][[:space:]]*["'\''][A-Za-z0-9/+_-]{20,}' \
        -e '-----BEGIN.*CERTIFICATE-----' \
        -- "$f"; then
        found=1
    fi
done

if [ "$found" -eq 1 ]; then
    echo "POTENTIAL SECRETS FOUND - review the matches above before committing"
    exit 1
fi

echo "no secret patterns detected"
exit 0

#!/usr/bin/env bash
# Localization Dictionary Validation Utility
# Contributed by: Mutheshkumar

set -e

echo "=== TierGuard Localization Dictionary Validation ==="

DASHBOARD_FILE="web/templates/dashboard.html"

if [ ! -f "$DASHBOARD_FILE" ]; then
    echo "❌ Error: $DASHBOARD_FILE not found."
    exit 1
fi

echo -n "Checking English (en) dictionary presence... "
if grep -q "title: \"SaaS Entitlements Engine\"" "$DASHBOARD_FILE"; then
    echo "✅ Present"
else
    echo "⚠️ Missing"
fi

echo -n "Checking Tamil (ta) dictionary presence... "
if grep -q "title: \"SaaS உரிமை மேலாண்மை இயங்குதளம்\"" "$DASHBOARD_FILE"; then
    echo "✅ Present"
else
    echo "⚠️ Missing"
fi

echo -n "Checking Hindi (hi) dictionary presence... "
if grep -q "title: \"SaaS पात्रता प्रबंधन इंजन\"" "$DASHBOARD_FILE"; then
    echo "✅ Present"
else
    echo "⚠️ Missing"
fi

echo "Localization dictionary validation complete."

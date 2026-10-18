# TierGuard Frontend Architecture & Trilingual Localization

## Overview
This guide documents the design principles, user interface architecture, HTMX interaction model, and trilingual internationalization system powering the **TierGuard Administrative Dashboard**.

---

## 1. Design System & Aesthetics
The frontend is engineered with the restraint, high information density, and product focus associated with enterprise developer platforms like Stripe and Linear.

### Core Visual Guidelines
- **Typography**: Inter via Google Fonts (`400`, `500`, `600`, `700`).
- **Layout**: High-density grid with sticky telemetry bar, status indicators, and compact tabular data.
- **Palette Tokens**: Tailored slate/zinc neutral tones, emerald connection status lights, and indigo primary action buttons.
- **Dark/Light Theme System**:
  - Class-based dark theme (`class="dark"` on `<html>`).
  - Stored in `localStorage.getItem('tierguard_theme')`.
  - Automatically matches system preference when no explicit choice is stored.

---

## 2. HTMX Integration & Zero Full Page Reloads
TierGuard uses HTMX (`unpkg.com/htmx.org@1.9.10`) for declarative AJAX interactions:

### Instant Tier Update Form
```html
<form hx-post="/admin/tenants/tier"
      hx-target="#tenants-table-container"
      hx-swap="innerHTML"
      hx-indicator="#refresh-spinner">
    <input type="hidden" name="tenant_id" value="{{.ID}}">
    <select name="tier_name" onchange="this.form.requestSubmit()">
        <!-- options -->
    </select>
</form>
```
- **Optimistic Partial Swapping**: Only the `#tenants-table-container` is re-rendered upon submission.
- **Duplication Safeguard**: Server detects `HX-Request: true` header to return partial inner table markup instead of the full layout document.

---

## 3. Trilingual Localization (i18n)
TierGuard supports on-the-fly language switching across **English**, **Tamil (தமிழ்)**, and **Hindi (हिन्दी)** with client-side translation dictionaries.

### Supported Locales
| Code | Language | Native Name |
|:---:|:---|:---|
| `en` | English | English |
| `ta` | Tamil | தமிழ் |
| `hi` | Hindi | हिन्दी |

### Key Dictionaries & Translation Map
```javascript
const translations = {
  en: {
    title: "SaaS Entitlements Engine",
    subtitle: "Real-time tenant quota metering & dynamic tier provisioning",
    tenants_title: "Active Tenant Directory",
    col_tenant: "Tenant",
    col_tier: "Active Tier",
    col_quota: "Monthly Quota Usage",
    col_actions: "Modify Tier"
  },
  ta: {
    title: "SaaS உரிமை மேலாண்மை இயங்குதளம்",
    subtitle: "நிகழ்நேர பயன்பாட்டு அளவீடு & உடனடி அடுக்கு மாற்றம்",
    tenants_title: "செயலில் உள்ள வாடிக்கையாளர்கள்",
    col_tenant: "வாடிக்கையாளர்",
    col_tier: "தற்போதைய அடுக்கு",
    col_quota: "மாத ஒதுக்கீடு பயன்பாடு",
    col_actions: "அடுக்கை மாற்று"
  },
  hi: {
    title: "SaaS पात्रता प्रबंधन इंजन",
    subtitle: "वास्तविक समय उपयोग मीटरिंग और तत्काल टियर प्रोविज़निंग",
    tenants_title: "सक्रिय ग्राहक निर्देशिका",
    col_tenant: "ग्राहक",
    col_tier: "सक्रिय टियर",
    col_quota: "मासिक कोटा उपयोग",
    col_actions: "टियर बदलें"
  }
};
```
- Stored in `localStorage.getItem('tierguard_lang')`.
- Dynamic DOM updating without page refresh via `data-i18n` attributes.

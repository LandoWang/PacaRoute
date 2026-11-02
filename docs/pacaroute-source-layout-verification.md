# PacaRoute 0.8.1 source dialog verification

## Reproduction and correction

- At 480 x 700, entering a 145-character model name without separators expanded
  the old fieldset to 1070.85px inside a 448px dialog. The repaired fieldset stays
  at 446px (the dialog's inner width), with the selected-model summary wrapping.
- Expanded advanced fields previously continued underneath the sticky footer.
  The new content scroller ends at the footer's top. At 480 x 700 both measured
  596px; the last Proxy field ends at 575.94px after scrolling to the bottom.
- Source/model route selectors previously measured 59.5px/40px, then
  72.30px/40px after a discovery error. Both now stay 40px high and share the
  same top coordinate, including the GitHub-not-logged-in error state.

## Browser checks

Used a separate Edge QA tab without saving source or route drafts. Checked:

- Source account selection and Azure add-source resource loading/selection.
- Expanded resource name/ID fields, no-deployments state and validation error.
- Existing Azure source editing, manual model input, advanced fields and Proxy.
- Long unbroken model text, 1424 x 798, 480 x 700 and 360 x 640 layouts.
- At 360 x 640 the dialog has zero horizontal overflow; validation text remains
  above the footer, which ends at 611px within the viewport.
- Shared route selector alignment before and after a discovery error.

Local screenshots and deployment metadata are under
`artifacts/source-layout-qa/` and are intentionally not committed. The existing
manually configured Foundry source has no subscription/resource metadata, so its
Discover Models action returns the existing missing-resource validation message;
this check verifies error layout, not successful discovery on that source.

## Build and deployment

- Web TypeScript/Vite production build and embed passed.
- Existing frontend tests: 10 passed across two files.
- Separate arm64/amd64 macOS desktop builds, ad-hoc signature checks and DMGs
  passed; Windows desktop/CLI and five CLI target builds passed.
- Web ZIP and distributions are in `artifacts/pacaroute-0.8.1-release/`.
- Replaced only the verified task-owned preview on port 63652. The management
  config response is identical before and after restart. The preview reports
  version 0.8.1 and serves the rebuilt assets.
- macOS arm64 desktop binary SHA-256:
  `caf16294b175eae54d57a4e99b5cda5afd9ca2a376946049df5f3978b2db975a`.

No protocol/routing behavior changed, and live Codex/Claude inference was not
rerun for this UI patch. Physical Intel/Windows execution, Safari and browser
zoom beyond the checked viewport sizes were not verified in this run.

## 0.8.2: stable source dialog dimensions

Follow-up feedback identified inconsistent outer dialog sizes. At 1424 x 798,
the account picker was 560 x 689 while the GitHub source form collapsed to
560 x 308.5; their vertical positions changed from 54.5px to 244.75px.

Source account selection, Azure discovery and source add/edit now opt into a
shared 560 x 720 dialog, bounded by the existing viewport margins. The body
fills the available space and scrolls, keeping the footer in place. Azure
discovery also has a description so its heading matches the other source steps.

Verified in Edge at 1424 x 798: account picker, GitHub collapsed/expanded, Azure
loading/loaded (189 resource options), and Azure edit collapsed/expanded all
measure 560 x 720 with a 39px top position. Editor footer tops remain at 683px.
GitHub advanced inputs and selects all measure 510 x 40. Repeated the edit check
against the packaged preview, which reports 0.8.2. The browser viewport override
did not apply during this follow-up, so new narrow-window measurements are not
claimed. The 0.8.1 checks above remain evidence for that earlier build only.

Production build/embed, macOS arm64/amd64, Windows desktop and five CLI target
builds passed. Packages are in `artifacts/pacaroute-0.8.2-release/`. The verified
task-owned preview was restarted on port 63652 with an identical config snapshot.
Local screenshot: `artifacts/source-layout-qa/consistent-size.png`.

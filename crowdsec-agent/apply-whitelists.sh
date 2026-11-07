#!/bin/sh
set -eu
# Runs before crowdsecurity/traefik-logs (lexical order) so comma-separated
# ClientHost values from websecure-cf still parse.
cp /runtime/traefik-client-host.yaml /etc/crowdsec/parsers/s01-parse/00-traefik-client-host.yaml
rm -f "/etc/crowdsec/postoverflows/s01-whitelist/01-my-whitelist.yaml"
if yq -e '.name != null' /runtime/dynamic-whitelist.yaml >/dev/null 2>&1; then
  cp /runtime/dynamic-whitelist.yaml "/etc/crowdsec/postoverflows/s01-whitelist/01-my-whitelist.yaml"
fi
set_whitelist() {
  setting=$1
  file=$2
  target=$3
  rm -f "/etc/crowdsec/$file"
  if [ "$(yq -r ".$setting.enabled" /runtime/whitelists.yaml)" = true ]; then
    ln -s "/etc/crowdsec/$target" "/etc/crowdsec/$file"
  fi
}
set_whitelist privateNetworks parsers/s02-enrich/whitelists.yaml hub/parsers/s02-enrich/crowdsecurity/whitelists.yaml
set_whitelist publicDNS parsers/s02-enrich/public-dns-allowlist.yaml hub/parsers/s02-enrich/crowdsecurity/public-dns-allowlist.yaml
set_whitelist cdn postoverflows/s01-whitelist/cdn-whitelist.yaml hub/postoverflows/s01-whitelist/crowdsecurity/cdn-whitelist.yaml
set_whitelist seoBots postoverflows/s01-whitelist/seo-bots-whitelist.yaml hub/postoverflows/s01-whitelist/crowdsecurity/seo-bots-whitelist.yaml
set_whitelist googleSpecialCrawlers postoverflows/s01-whitelist/google-special-crawlers-whitelist.yaml hub/postoverflows/s01-whitelist/crowdsecurity/google-special-crawlers-whitelist.yaml

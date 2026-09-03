// ---- IPv4 ----

const octet = String.raw`([1-9]?[0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])`
const cidr4 = String.raw`([1-2]?[0-9]|3[0-2])`

const IPv4noCIDR = String.raw`(${octet}(\.${octet}){3})`
const IPv4CIDR = String.raw`(${IPv4noCIDR}/${cidr4})`
const IPv4 = String.raw`(${IPv4noCIDR}(/${cidr4})?)`

// ---- IPv6 ----

const segment = String.raw`([0-9a-fA-F]{1,4})`
const cidr6 = String.raw`([1-9]?[0-9]|1[0-1][0-9]|12[0-8])`

const sm1 = String.raw`(|${segment})` // segments, max 1 (may be 0)
const sm2 = String.raw`(|${segment}(:${segment}){0,1})`
const sm3 = String.raw`(|${segment}(:${segment}){0,2})`
const sm4 = String.raw`(|${segment}(:${segment}){0,3})`
const sm5 = String.raw`(|${segment}(:${segment}){0,4})`
const sm6 = String.raw`(|${segment}(:${segment}){0,5})`
const sm7 = String.raw`(|${segment}(:${segment}){0,6})`

const sexa8 = String.raw`(${segment}(:${segment}){7})` // segments, exactly 8
const sexa6 = String.raw`(${segment}(:${segment}){5})`

const IPv6General = String.raw`(${sexa8}|::${sm7}|${sm1}::${sm6}|${sm2}::${sm5}|${sm3}::${sm4}|${sm4}::${sm3}|${sm5}::${sm2}|${sm6}::${sm1}|${sm7}::)`

const IPv4inV6Part1 = String.raw`(${sexa6}:|::${sm5}:|${sm1}::${sm4}:|${sm2}::${sm3}:|${sm3}::${sm2}:|${sm4}::${sm1}:|${sm5}::)`
const IPv4inV6 = String.raw`(${IPv4inV6Part1}${IPv4noCIDR})`

const IPv6WithZone = String.raw`(${IPv6General}%.+)`

const IPv6noCIDR = String.raw`(${IPv6General}|${IPv4inV6}|${IPv6WithZone})`
const IPv6CIDR = String.raw`(${IPv6noCIDR}/${cidr6})`
const IPv6 = String.raw`(${IPv6noCIDR}(/${cidr6})?)`

// ---- other ----

const IPv4Range = String.raw`(${IPv4noCIDR}-${IPv4CIDR})`
const IPv6Range = String.raw`(${IPv6noCIDR}-${IPv6CIDR})`

export const Patterns = {
    wholeNumber: /^\d+$/,
    IPv4: new RegExp("^" + IPv4 + "$"),
    IPv6: new RegExp("^" + IPv6 + "$"),
    IPv4noCIDR: new RegExp("^" + IPv4noCIDR + "$"),
    IPv6noCIDR: new RegExp("^" + IPv6noCIDR + "$"),
    IPv4CIDR: new RegExp("^" + IPv4CIDR + "$"),
    IPv6CIDR: new RegExp("^" + IPv6CIDR + "$"),
    IPv4Range: new RegExp("^" + IPv4Range + "$"),
    IPv6Range: new RegExp("^" + IPv6Range + "$"),
    macAddress: /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/,
    portNumber:
        /^([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5])$/,
};

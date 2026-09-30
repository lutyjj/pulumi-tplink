// Intermediate representation shared by the extract -> spec -> emit stages.
//
// Deliberately vendor-neutral in shape: an endpoint is a (namespace, form) pair
// addressed by POST, and every call carries an `operation` discriminator. Nothing
// here knows about DHCP, wireless, or any particular router feature.

/** One `operation=` value an endpoint accepts. */
export interface EndpointOperation {
    /** Wire value of the `operation` form field (`load`, `insert`, `loadDevice`, …). */
    operation: string;
    /** Additional form fields observed alongside it at the call site. */
    requestFields: string[];
}

/** A single `<path>?form=<form>` endpoint and everything known about it. */
export interface Endpoint {
    /**
     * Path exactly as the UI writes it, minus any leading slash — `admin/dhcps`,
     * `login`. Authenticated endpoints live under `admin/`; the login handshake
     * does not, so the prefix is load-bearing when rebuilding a URL.
     */
    path: string;
    /** Last path segment: `dhcps`, `login`. Used for grouping only. */
    namespace: string;
    form: string;
    operations: EndpointOperation[];
    /** Bundle chunks the endpoint was seen in — provenance for re-verification. */
    chunks: string[];
}

/**
 * A wire payload shape recovered from a view-model -> request mapper. Not yet
 * bound to an endpoint: association is what the overlay does.
 */
export interface WireShape {
    fields: string[];
    chunk: string;
}

export interface ExtractedApi {
    /** Firmware string from the UI's `<meta name="version">`, when captured. */
    firmware?: string;
    endpoints: Endpoint[];
    shapes: WireShape[];
}

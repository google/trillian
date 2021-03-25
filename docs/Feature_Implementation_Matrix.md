# Trillian Feature/Implementation Matrix

 - [Overview](#overview)
 - [Functionality](#functionality)
   - [Log v1](#trillian-log-v1)
   - [Log v2](#trillian-log-v2-skylog)
 - [Concrete implementations](#concrete-implementations)
   - [Storage](#storage)
   - [Monitoring](#monitoring)
   - [Master election](#master-election)
   - [Quota](#quota)


## Overview

This page summarises the various features which are present in Trillian, and
their implementation status.

The status of features is listed as one of:
  * Not Implemented (NI)
  * In Development
  * Partial
  * Alpha
  * Beta
  * General Availability (GA)
  * Deprecated (⚠)


## Functionality

|                     |  Status            | Deployed in production    | Notes                                                   |
|:---                  |   :---:           | :---:                      |:---                                                     |
| Log V1               |   GA              | ✓                         |                                                         |

### Trillian Log V1

This is feature complete, and is actively used in production by multiple CT log operators, including Google.

### Trillian Log V2 (Skylog)

Skylog is an append-only log with considerably higher throughput and lower integration latency than the v1 log.
It currently exists as internal prototype.

## Concrete implementations

This section lists the status of implementations for the _pluggable_ subsystems which Trillian supports.

### Storage

Trillian supports "pluggable" storage implementations for durable storage of the merkle tree data.
The state and characteristics of these implementations are detailed below.

#### V1 log storage

The Log storage implementations supporting the original Trillian log.


| Storage          | Status  | Deployed in prod    | Notes                                                                       |
|:---              | :---:   | :---:                |:---                                                                         |
| Spanner          | GA      | ✓                   | Google internal-only, see CloudSpanner for external use.                    |
| CloudSpanner    | Beta     |                     | Google maintains continuous-integration environment based on CloudSpanner.  |
| MySQL            | GA      | ✓                   |                                                                             |

##### Spanner
This is a Google-internal implementation, and is used by all of Google's current Trillian deployments.

##### CloudSpanner
This implementation uses the Google CloudSpanner APIs in GCE.
It's been tested to tens of billions of entries and tens of log tenants.

Performance largely depends on the number of CloudSpanner servers allocated,
but write throughput of 1000+ entries/s has been observed.

[Issue #1681](https://github.com/google/trillian/issues/1681) tracks this becoming ready for GA.

##### MySQL
This implementation has been tested with MySQL 5.7.
It's currently in production use by at least one CT log operator.

Write throughput of 4-500 entries/s has been observed.

### Monitoring

Supported monitoring frameworks, allowing for production monitoring and alerting.

| Monitoring      | Status  | Deployed in prod    | Notes                                                                       |
|:---             | :---:   | :---:               |:---                                                                         |
| Prometheus      | GA      | ✓                   |                                                                             |
| OpenCensus      | Partial |                     | Currently, only support for Tracing is implemented.                         |

### Master election

Supported frameworks for providing Master Election.

| Election        | Status  | Deployed in prod    | Notes                                                                       |
|:---             | :---:   | :---:               |:---                                                                         |
| Chubby          | GA      | ✓                   | Google internal-only.                                                       |
| etcd            | GA      | ✓                   |                                                                             |

### Quota

Supported frameworks for quota management.

| Implementation  | Status  | Deployed in prod    | Notes                                                                       |
|:---             | :---:   | :---:               |:---                                                                         |
| Google internal | GA      | ✓                   |                                                                             |
| etcd            | GA      | ✓                   |                                                                             |
| MySQL           | Beta    | ?                   |                                                                             |
| Redis           | Alpha   | ✓                   |                                                                             |

### Key management

Supported frameworks for key management and signing.

| Election        | Status  | Deployed in prod    | Notes                                                                       |
|:---             | :---:   | :---:               |:---                                                                         |
| Google internal | GA      | ✓                   |                                                                             |
| golang stdlib   | GA      |                     | i.e PEM files, etc.                                                         |
| PKCS#11         | GA      | ?                   |                                                                             |

import type {FirmwareCandidateTrust} from './firmwareCandidate.ts';

// Qualified custom distributions can provision the independently verified board
// revision and public pins here before signing the app. The default distribution
// has no qualified candidate authority.
export const firmwareCandidateDeployment:FirmwareCandidateTrust|undefined=undefined;

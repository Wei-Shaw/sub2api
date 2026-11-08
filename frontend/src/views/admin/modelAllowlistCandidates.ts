import type { GroupPlatform } from "@/types";

export type ModelAllowlistCandidatesMode = "create" | "edit";

export interface ModelAllowlistCandidatesRequest {
  mode: ModelAllowlistCandidatesMode;
  groupID: number;
  platform: GroupPlatform;
}

export interface ModelAllowlistCandidatesTracker {
  next(request: ModelAllowlistCandidatesRequest): number;
  isCurrent(requestID: number, request: ModelAllowlistCandidatesRequest): boolean;
}

export const createModelAllowlistCandidatesTracker = (): ModelAllowlistCandidatesTracker => {
  let currentRequestID = 0;
  const currentByMode: Partial<Record<ModelAllowlistCandidatesMode, {
    id: number;
    request: ModelAllowlistCandidatesRequest;
  }>> = {};

  return {
    next(request) {
      currentRequestID += 1;
      currentByMode[request.mode] = {
        id: currentRequestID,
        request: { ...request },
      };
      return currentRequestID;
    },
    isCurrent(requestID, request) {
      const current = currentByMode[request.mode];
      return (
        current?.id === requestID &&
        current.request.groupID === request.groupID &&
        current.request.platform === request.platform
      );
    },
  };
};

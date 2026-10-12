import type { GroupPlatform } from "@/types";

export type ModelsListCandidatesMode = "create" | "edit";

export interface ModelsListCandidatesRequest {
  mode: ModelsListCandidatesMode;
  groupID: number;
  platform: GroupPlatform;
}

export interface ModelsListCandidatesTracker {
  next(request: ModelsListCandidatesRequest): number;
  isCurrent(requestID: number, request: ModelsListCandidatesRequest): boolean;
}

export const createModelsListCandidatesTracker = (): ModelsListCandidatesTracker => {
  let currentRequestID = 0;
  const currentByMode: Partial<Record<ModelsListCandidatesMode, {
    id: number;
    request: ModelsListCandidatesRequest;
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

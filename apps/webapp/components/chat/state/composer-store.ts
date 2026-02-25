"use client";

import { create } from "zustand";
import type { ModelSelection } from "@protean/model-catalog";
import type { ComposerState } from "@/components/chat/state/types";

interface ComposerStore extends ComposerState {
  enforceModelReasoningBudget: (args: {
    budgets: string[];
    defaultBudget: ModelSelection["reasoningBudget"];
  }) => void;
  hydrateComposer: (args: {
    deepResearchEnabled?: boolean;
    imageCreationEnabled?: boolean;
    modelSelection: ModelSelection;
  }) => void;
  setModelSelection: (selection: ModelSelection) => void;
  setReasoningBudget: (budget: string) => void;
  toggleDeepResearch: (enabled: boolean) => void;
  toggleImageCreation: (enabled: boolean) => void;
}

const emptyModelSelection: ModelSelection = {
  modelId: "",
  providerId: "",
  reasoningBudget: "none",
  runtimeProvider: "",
};

const initialState: ComposerState = {
  deepResearchEnabled: false,
  imageCreationEnabled: false,
  modelSelection: emptyModelSelection,
};

export const useComposerStore = create<ComposerStore>()((set) => ({
  ...initialState,

  enforceModelReasoningBudget: ({ budgets, defaultBudget }) =>
    set((state) => {
      if (budgets.includes(state.modelSelection.reasoningBudget)) {
        return state;
      }

      return {
        modelSelection: {
          ...state.modelSelection,
          reasoningBudget: defaultBudget,
        },
      };
    }),

  hydrateComposer: ({
    deepResearchEnabled,
    imageCreationEnabled,
    modelSelection,
  }) =>
    set((state) => ({
      deepResearchEnabled: deepResearchEnabled ?? state.deepResearchEnabled,
      imageCreationEnabled: imageCreationEnabled ?? state.imageCreationEnabled,
      modelSelection,
    })),

  setModelSelection: (modelSelection) => set({ modelSelection }),

  setReasoningBudget: (budget) =>
    set((state) => ({
      modelSelection: {
        ...state.modelSelection,
        reasoningBudget: budget as ModelSelection["reasoningBudget"],
      },
    })),

  toggleDeepResearch: (deepResearchEnabled) => set({ deepResearchEnabled }),

  toggleImageCreation: (imageCreationEnabled) => set({ imageCreationEnabled }),
}));

import { ThreadRouteContent } from "@/components/chat/thread-route-content";
import {
  getDefaultModelSelection,
  getModelCatalog,
} from "@protean/model-catalog";

export default function NewChatPage() {
  const providers = getModelCatalog();
  const defaultModelSelection = getDefaultModelSelection();

  return (
    <ThreadRouteContent
      defaultModelSelection={defaultModelSelection}
      providers={providers}
    />
  );
}

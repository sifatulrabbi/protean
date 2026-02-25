"use client";

import { FolderOpenIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useSidebar } from "@/components/ui/sidebar";

export function ThreadHeader() {
  const { toggleSidebar } = useSidebar();

  return (
    <>
      {/* Mobile only: toggle button for the files sheet */}
      <div className="fixed top-3 right-3 z-40 lg:hidden">
        <Button size="icon" variant="outline" onClick={toggleSidebar}>
          <FolderOpenIcon className="size-5" />
          <span className="sr-only">Files</span>
        </Button>
      </div>
    </>
  );
}

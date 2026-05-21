"use client";

import { useEffect, useId, useMemo } from "react";
import Uppy from "@uppy/core";
import Tus from "@uppy/tus";
import Dashboard from "@uppy/dashboard";
import "@uppy/core/css/style.min.css";
import "@uppy/dashboard/css/style.min.css";

type Props = {
  folderId: string | null;
  onComplete?: () => void;
};

export function TusUploader({ folderId, onComplete }: Props) {
  const dashboardTarget = useId().replace(/:/g, "_");

  const uppy = useMemo(() => {
    const instance = new Uppy({
      autoProceed: false,
      restrictions: {
        maxNumberOfFiles: 50,
      },
      allowMultipleUploadBatches: true,
      meta: {
        folderid: folderId ?? "",
      },
    });

    instance.use(Tus, {
      endpoint: "/api/uploads/tus",
      chunkSize: 5 * 1024 * 1024,
      withCredentials: true,
      removeFingerprintOnSuccess: false,
      allowedMetaFields: ["name", "filename", "folderid"],
      headers: {
        "Tus-Resumable": "1.0.0",
      },
    });

    instance.use(Dashboard, {
      inline: true,
      target: `#${dashboardTarget}`,
      height: 330,
      proudlyDisplayPoweredByUppy: false,
      showLinkToFileUploadResult: false,
      note: "Resumable uploads via Tus protocol",
    });

    return instance;
  }, [dashboardTarget]);

  useEffect(() => {
    uppy.setMeta({ folderid: folderId ?? "" });
  }, [uppy, folderId]);

  useEffect(() => {
    const handleComplete = () => {
      onComplete?.();
    };
    uppy.on("complete", handleComplete);

    return () => {
      uppy.off("complete", handleComplete);
      uppy.destroy();
    };
  }, [onComplete, uppy]);

  return <div id={dashboardTarget} />;
}

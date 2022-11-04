/** @xstate-layout N4IgpgJg5mDOIC5QFEC2BDAlgGwAQDUwAnTAM0wGN0AXTAewDsA6AVQYDdizNIBiAYSJgaYXC1jFcAdUzUAFrjRZsAGkUYcBLuSq1GuADKYGAa0UAPAA6YhEANoAGALqJQlurFn0GrkOcQAjADsDkwATAAcAJwOQXEArBFB0Q4AbCogAJ6IEQDMAL75GUqahCQ6NN6sHNo8EAJCImISRNKyCiWqWuWUlfpGpgTo2Jj2zr7unno+SH6BIeEpcUGJyTHpWYEALAWFGQx0EHC+nd3culVsnD2QEx5ejL7+CFthGdkIMUwBqb9--78gnsQKcyuc+swAGLKW6zSYPGagZ6pAIBJhbVIOKLxMIrJLRALvHK5JjxYGg2oXRhMMHkWFue7TJ6IFFojFYnF4taEzYIAI7UnkjR4Wm9JlwxneZkIAC0Gw+csKhSAA */
createMachine({
  context: {},
  schema: {
    context: {} as {},
    events: {} as
      | { type: "Create User With Email, Email Verification Link Expired" }
      | { type: "Create User With Email, Verification Link Valid" },
  },
  preserveActionOrder: true,
  predictableActionArguments: true,
  id: "Email Verification",
  initial: "Unverified",
  states: {
    Unverified: {
      on: {
        "Create User With Email, Email Verification Link Expired": {
          target: "Failed",
        },
        "Create User With Email, Verification Link Valid": {
          target: "Verified",
        },
      },
    },
    Failed: {},
    Verified: {},
  },
});

/** @xstate-layout N4IgpgJg5mDOIC5QFEC2BDAlgGwAQDUwAnTAM0wGN0AXTAewDsA6AVQYDdizNIBiAYSJgaYXC1jFcAdUzUAFrjRZsAGkUYcBLuSq1GuADKYGAa0UAPAA6YhEANoAGALqJQlurFn0GrkOcQAjABsAMxMDg4ArACcDiEBkQ4ATAEALJFJKiAAnoghQUyRiQDs0RlB0aWpCQC+NVlKmoQkOjTerBzaPBACQiJiEkTSsgqNqlotlG36RqYE6NiY9s6+7p56Pkh+gcUOTEHFqcXx0RUAHCGXxVm5CAlJ+6VJZ2dHxQHR1Ul1DRp4zdxdO0AGLKPgsSwQfpgP6OFxbNZeRi+fwIS4FaIhIqREJnIKpM6lM43RAAWkuTBS6VSSWKQRxF1SPxAYwmgOmzAB5HBkP6Yzhqw8SM2oFRkVehRK0U+NIclUyOUQSRS4Xe0Vp8UiqQcAQCITq9RADDoEDgvlZXKmGw6nEmkEF628KMQNJJCFiTGCQW9Pt9B2ZFq6QMYTFBOHtCKFG2dCCCuqYqSCcoyxXFxTO0QCbouhQDfzZrWtlojbijTq2qLjAQTSbKtLTGaziruqTCkTzygLVvLpcdyIrZKCbtJAWVlLSGTKkVTDm17YNQA */
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
    Failed: {
      on: {
        "Update email": {
          target: "Unverified",
        },
      },
    },
    Verified: {
      on: {
        "Update Email": {
          target: "Unverified",
        },
      },
    },
  },
});

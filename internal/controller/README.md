# Torchrun Controller Refactoring

This directory contains the refactored TorchrunJob controller, which has been broken down into multiple files and organized into logical folders for better maintainability and organization.

## File Structure

### Core Controller

- **`torchrunjob/controller.go`** (108 lines) - Main reconciliation logic and controller setup
  - Contains the `TorchrunJobReconciler` struct and `Reconcile` method
  - Orchestrates the workflow between different managers
  - Handles the main reconciliation loop

### Manager Components

#### Job Management

- **`torchrunjob/job_manager.go`** (305 lines) - Kubernetes Job creation and management
  - `JobManager` struct and `CreateJob` method
  - Torchrun command building and pod specification
  - Volume and environment attachment
  - Pod validation and labeling

#### Workspace Management

- **`torchrunjob/workspace_manager.go`** (304 lines) - Workspace-related operations
  - `WorkspaceManager` struct for PVC and sync pod management
  - Workspace PVC creation and status checking
  - Sync pod creation and command building
  - Support for different workspace sources (zip, git, s3)

#### Status Management

- **`torchrunjob/status_manager.go`** (108 lines) - Status updates and condition management
  - `StatusManager` struct for status updates
  - Phase determination based on Kubernetes Job status
  - Condition management and updates

### Utilities

- **`utils.go`** (23 lines) - Helper functions
  - Pointer creation utilities (`quantityPtr`, `boolPtr`, `completionModePtr`)
  - Common helper functions used across managers

### Compatibility Layer

- **`torchrunjob.go`** (25 lines) - Backward compatibility functions
  - `NewTorchrunJobReconciler()` - Creates TorchrunJobReconciler instances
  - `NewJobQueueReconciler()` - Creates JobQueueReconciler instances
  - Maintains compatibility with main.go without changing its imports

### Other Controllers

- **`jobqueue/controller.go`** (248 lines) - JobQueue controller (unchanged)

## Directory Organization

```
internal/controller/
├── README.md                    # This documentation
├── utils.go                     # Shared utilities
├── torchrunjob.go               # Compatibility layer for main.go
├── torchrunjob/                 # TorchrunJob controller components
│   ├── controller.go            # Main controller logic
│   ├── job_manager.go           # Job creation and management
│   ├── workspace_manager.go     # Workspace operations
│   └── status_manager.go        # Status and condition management
└── jobqueue/                    # JobQueue controller
    └── controller.go            # JobQueue controller logic
```

## Benefits of Refactoring

1. **Improved Readability**: Each file has a single, clear responsibility
2. **Better Maintainability**: Changes to specific functionality are isolated
3. **Easier Testing**: Individual managers can be tested in isolation
4. **Reduced Complexity**: The main controller file is now focused on orchestration
5. **Better Organization**: Related functionality is grouped together in logical folders
6. **Clear Separation**: Different controller types are separated into their own directories
7. **Backward Compatibility**: Main.go continues to work without changes

## Architecture

The refactored controller follows a manager pattern where:

1. **Main Controller** (`torchrunjob/controller.go`) orchestrates the workflow
2. **Workspace Manager** (`torchrunjob/workspace_manager.go`) handles workspace preparation and synchronization
3. **Job Manager** (`torchrunjob/job_manager.go`) creates and manages the Kubernetes Job
4. **Status Manager** (`torchrunjob/status_manager.go`) updates the TorchrunJob status and conditions

Each manager is responsible for its own domain and can be developed/tested independently.

## Usage

The refactored controller maintains the same external API and behavior as the original monolithic controller. The changes are internal and improve code organization without affecting functionality.

### For main.go

The main.go file continues to work exactly as before:

```go
// These lines work unchanged
if err = controller.NewTorchrunJobReconciler(
    mgr.GetClient(),
    mgr.GetScheme(),
).SetupWithManager(mgr); err != nil {
    // ...
}

if err = controller.NewJobQueueReconciler(
    mgr.GetClient(),
    mgr.GetScheme(),
).SetupWithManager(mgr); err != nil {
    // ...
}
```

## Import Structure

The new folder structure allows for cleaner imports and better separation of concerns:

- `torchrunjob/` - All TorchrunJob-related functionality
- `jobqueue/` - JobQueue controller functionality
- `utils.go` - Shared utilities available to all controllers
- `torchrunjob.go` - Compatibility layer for external consumers

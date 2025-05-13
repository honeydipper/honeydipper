# FAQ

## General questions

### What is Honeydipper?
Honeydipper is an event-driven, policy-based orchestration system that, is tailored towards SREs and DevOps workflows, and has a pluggable open architecture. The purpose is to fill the gap between the various components used in DevOps operations, to act as an orchestration hub, and to replace the ad-hoc integrations between the components so that the all the integrations can also be composed as code.

### What is DipperCL?
DipperCL stands for Dipper Control Language. It is a yaml based language used for configuring Honeydipper, and defining assets within Honeydipper.

### What are the assets used in Honeydipper?
Honeydipper uses 4 type of assets. They are driver, system, workflow and rules.

### What is a driver?
Honeydipper driver is a golang program that is dynamically loaded by the daemon to perform various tasks or ingest events. For example, webhook driver, web driver, kubernetes driver, etc.

### What is a system?
A system is an abstract representation of a physical system in Honeydipper. It is defined by a group triggers and functions through which the system interacts with Honeydipper and the world. For example, github system has a group of triggers implemented through webhook driver, and a group of functions that calls github APIs through web the driver.

### What is a workflow?
A workflow is a data structure that defines the work needed to complete a task. It can contains steps, threads or invoking functions, other workflows. It can also contain definition of context variables, and evaluate conditions before taking actions.

### What is a rule?
A rule defines a trigger, a set of conditions and a workflow indicating that the workflow needs to be executed when the trigger is fired and the conditions are met.
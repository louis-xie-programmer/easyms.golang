# Database Package

This package provides database access functionality for the EasyMS microservice framework.

## Components

1. **Core Database Interface** - Standard interface for database operations
2. **Database Factory** - Factory pattern implementation for creating database instances
3. **Database Implementations** - Specific implementations for different database types
4. **Array Types** - Special types for handling database-specific array types
5. **Redis Cache** - Redis client wrapper with caching features

## Features

- Support for multiple database types (MySQL, PostgreSQL, SQL Server)
- Connection pooling management
- Automatic migration support
- PostgreSQL array type handling
- Redis caching with protection mechanisms
- Unified database access interface
-- Banco "Andino del Quindío" — esquema demo para la DBaaS.
-- Compatible con MariaDB y PostgreSQL: usa IDs explícitos para evitar la
-- divergencia AUTO_INCREMENT (MariaDB) vs SERIAL/IDENTITY (PostgreSQL).

CREATE TABLE sucursales (
    id          INT PRIMARY KEY,
    nombre      VARCHAR(80)  NOT NULL UNIQUE,
    ciudad      VARCHAR(60)  NOT NULL,
    direccion   VARCHAR(150),
    telefono    VARCHAR(20)
);

CREATE TABLE empleados (
    id              INT PRIMARY KEY,
    sucursal_id     INT NOT NULL REFERENCES sucursales(id),
    nombre          VARCHAR(120) NOT NULL,
    cargo           VARCHAR(60)  NOT NULL,           -- cajero | asesor | gerente | analista
    email           VARCHAR(120) UNIQUE,
    telefono        VARCHAR(20),
    fecha_ingreso   DATE NOT NULL
);

CREATE TABLE clientes (
    id                INT PRIMARY KEY,
    documento         VARCHAR(20) NOT NULL UNIQUE,    -- cédula o NIT
    nombre            VARCHAR(120) NOT NULL,
    email             VARCHAR(120) UNIQUE,
    telefono          VARCHAR(20),
    ciudad            VARCHAR(60),
    fecha_registro    DATE NOT NULL
);

CREATE TABLE cuentas (
    id              INT PRIMARY KEY,
    cliente_id      INT NOT NULL REFERENCES clientes(id),
    sucursal_id     INT NOT NULL REFERENCES sucursales(id),
    numero          VARCHAR(20) NOT NULL UNIQUE,
    tipo            VARCHAR(20) NOT NULL,             -- ahorros | corriente | cdt
    saldo           DECIMAL(14,2) NOT NULL DEFAULT 0,
    fecha_apertura  DATE NOT NULL,
    estado          VARCHAR(20) NOT NULL DEFAULT 'activa'
);

CREATE TABLE transacciones (
    id              INT PRIMARY KEY,
    cuenta_id       INT NOT NULL REFERENCES cuentas(id),
    fecha           DATE NOT NULL,
    tipo            VARCHAR(20) NOT NULL,             -- deposito | retiro | transferencia | pago_servicio
    monto           DECIMAL(14,2) NOT NULL,
    descripcion     VARCHAR(200),
    cuenta_destino  INT REFERENCES cuentas(id)        -- usada solo en transferencias
);

CREATE TABLE prestamos (
    id                  INT PRIMARY KEY,
    cliente_id          INT NOT NULL REFERENCES clientes(id),
    sucursal_id         INT NOT NULL REFERENCES sucursales(id),
    monto               DECIMAL(14,2) NOT NULL,
    tasa_interes        DECIMAL(5,2) NOT NULL,        -- porcentaje efectivo anual
    plazo_meses         INT NOT NULL,
    fecha_aprobacion    DATE NOT NULL,
    estado              VARCHAR(20) NOT NULL DEFAULT 'vigente'  -- vigente | pagado | mora
);

-- Sucursales
INSERT INTO sucursales (id, nombre, ciudad, direccion, telefono) VALUES
    (1, 'Sucursal Centro Armenia',  'Armenia',     'Cra 14 #20-45',     '+57 6 7445500'),
    (2, 'Sucursal Pinares',         'Pereira',     'Av. Circunvalar 12-08', '+57 6 3219090'),
    (3, 'Sucursal Cable',           'Manizales',   'Cra 23 #62-30',     '+57 6 8801122'),
    (4, 'Sucursal Norte Cali',      'Cali',        'Av. 6N #28-50',     '+57 2 5567788'),
    (5, 'Sucursal Chapinero',       'Bogota',      'Cl 67 #11-30',      '+57 1 4488221');

-- Empleados
INSERT INTO empleados (id, sucursal_id, nombre, cargo, email, telefono, fecha_ingreso) VALUES
    (201, 1, 'Hernan Botero',     'gerente',  'hernan.botero@andino.co',  '+57 300 555 0102', '2019-03-12'),
    (202, 1, 'Luisa Mosquera',    'asesor',   'luisa.mosquera@andino.co', '+57 311 555 0144', '2021-06-01'),
    (203, 2, 'Andres Castano',    'gerente',  'andres.castano@andino.co', '+57 320 555 0178', '2018-10-15'),
    (204, 2, 'Mei Lin',           'analista', 'mei.lin@andino.co',        '+57 301 555 0210', '2022-02-20'),
    (205, 3, 'Carolina Trujillo', 'cajero',   'carolina.t@andino.co',     '+57 314 555 0356', '2023-01-09'),
    (206, 4, 'Diego Patino',      'asesor',   'diego.patino@andino.co',   '+57 301 555 0467', '2020-08-03'),
    (207, 5, 'Elena Cardona',     'gerente',  'elena.cardona@andino.co',  '+57 314 555 0820', '2017-05-22');

-- Clientes
INSERT INTO clientes (id, documento, nombre, email, telefono, ciudad, fecha_registro) VALUES
    (1, '1094567890', 'Andrea Lopez',      'andrea.lopez@correo.co',   '+57 300 555 0142', 'Armenia',    '2022-04-18'),
    (2, '1088123456', 'Bruno Mejia',       'bruno.mejia@correo.co',    '+57 311 555 0098', 'Pereira',    '2021-11-30'),
    (3, '1075987654', 'Carolina Trujillo', 'caro.trujillo@correo.co',  '+57 320 555 0011', 'Manizales',  '2023-02-14'),
    (4, '1098765432', 'Diego Patino',      'diego.patino.cli@correo.co','+57 301 555 0467', 'Armenia',   '2020-09-05'),
    (5, '1056432198', 'Elena Cardona',     'elena.cardona.cli@correo.co','+57 314 555 0820','Cali',      '2019-07-22'),
    (6, '900123456-1','Cafe del Quindio SAS','contacto@cafedelquindio.co','+57 6 7445599', 'Armenia',    '2018-06-10');

-- Cuentas
INSERT INTO cuentas (id, cliente_id, sucursal_id, numero, tipo, saldo, fecha_apertura, estado) VALUES
    (5001, 1, 1, '0011-0001-2233', 'ahorros',    4520000.00, '2022-04-18', 'activa'),
    (5002, 1, 1, '0011-0002-3344', 'corriente',  1280000.00, '2023-01-10', 'activa'),
    (5003, 2, 2, '0022-0001-1155', 'ahorros',    1875000.00, '2021-12-01', 'activa'),
    (5004, 3, 3, '0033-0001-7788', 'ahorros',     680000.00, '2023-02-14', 'activa'),
    (5005, 4, 1, '0011-0003-9988', 'corriente',  9320000.00, '2020-09-06', 'activa'),
    (5006, 5, 4, '0044-0001-1212', 'ahorros',    3150000.00, '2019-07-23', 'activa'),
    (5007, 5, 4, '0044-0002-3434', 'cdt',       15000000.00, '2025-03-01', 'activa'),
    (5008, 6, 1, '0011-0004-5566', 'corriente', 42500000.00, '2018-06-12', 'activa'),
    (5009, 6, 5, '0055-0001-7878', 'ahorros',    2100000.00, '2024-10-04', 'inactiva');

-- Transacciones (los montos no recalculan el saldo; son histórico ilustrativo)
INSERT INTO transacciones (id, cuenta_id, fecha, tipo, monto, descripcion, cuenta_destino) VALUES
    (90001, 5001, '2026-04-22', 'deposito',        1200000.00, 'Consignacion nomina',                    NULL),
    (90002, 5001, '2026-04-25', 'retiro',           300000.00, 'Retiro cajero centro Armenia',           NULL),
    (90003, 5002, '2026-04-28', 'pago_servicio',    180000.00, 'Pago servicios publicos abril',          NULL),
    (90004, 5003, '2026-05-01', 'deposito',         950000.00, 'Pago freelance',                         NULL),
    (90005, 5003, '2026-05-03', 'transferencia',    500000.00, 'Transferencia interna a Andrea Lopez',  5001),
    (90006, 5005, '2026-05-05', 'transferencia',   2500000.00, 'Pago proveedor Cafe del Quindio',       5008),
    (90007, 5006, '2026-05-09', 'deposito',        1395000.00, 'Consignacion nomina',                    NULL),
    (90008, 5004, '2026-05-10', 'retiro',           120000.00, 'Retiro cajero El Cable',                 NULL),
    (90009, 5008, '2026-05-11', 'pago_servicio',    480000.00, 'Impuesto industria y comercio',          NULL),
    (90010, 5002, '2026-05-13', 'transferencia',    580000.00, 'Pago arriendo oficina',                 5005),
    (90011, 5007, '2026-05-15', 'deposito',       15000000.00, 'Apertura CDT 90 dias',                   NULL);

-- Prestamos
INSERT INTO prestamos (id, cliente_id, sucursal_id, monto, tasa_interes, plazo_meses, fecha_aprobacion, estado) VALUES
    (60001, 1, 1,  8000000.00, 22.50, 36, '2024-05-10', 'vigente'),
    (60002, 2, 2, 15000000.00, 19.80, 60, '2023-08-22', 'vigente'),
    (60003, 4, 1,  4500000.00, 24.00, 24, '2022-11-15', 'pagado'),
    (60004, 6, 1, 60000000.00, 14.50, 48, '2024-01-30', 'vigente'),
    (60005, 3, 3,  2500000.00, 26.30, 12, '2025-09-05', 'mora');

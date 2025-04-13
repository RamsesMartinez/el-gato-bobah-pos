import React from 'react';
import { 
  Breadcrumb as ChakraBreadcrumb,
  BreadcrumbItem as ChakraBreadcrumbItem,
  BreadcrumbLink,
  Text
} from '@chakra-ui/react';
import { ChevronRightIcon } from '@chakra-ui/icons';
import { Link as RouterLink } from 'react-router-dom';
import { BreadcrumbProps } from '../../types/breadcrumb';

export const Breadcrumb: React.FC<BreadcrumbProps> = ({ items }) => {
  return (
    <ChakraBreadcrumb
      spacing="8px"
      separator={<ChevronRightIcon color="gray.500" />}
    >
      {items.map((item, index) => (
        <ChakraBreadcrumbItem key={item.label} isCurrentPage={index === items.length - 1}>
          {item.href ? (
            <BreadcrumbLink 
              as={RouterLink} 
              to={item.href}
              color="blue.500"
              _hover={{ textDecoration: 'none', color: 'blue.600' }}
            >
              {item.label}
            </BreadcrumbLink>
          ) : (
            <Text color="gray.700" fontWeight="medium">
              {item.label}
            </Text>
          )}
        </ChakraBreadcrumbItem>
      ))}
    </ChakraBreadcrumb>
  );
}; 